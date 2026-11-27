package analysis

import (
	"fmt"
	"math"
	"math/cmplx"

	"resampleforensics/internal/dsp"
)

type transferResult struct {
	h           []complex128
	coherence   []float64
	valid       []int
	phase       []float64
	fftSize     int
	frames      int
	slope       float64
	intercept   float64
	fitDelayMS  float64
	phaseRMSDeg float64
	rippleMS    float64
	medianCoh   float64
	reliable    bool
}

func estimateTransfer(ref, dst []float64, rate int, opts Options, integerLag int) (transferResult, []string, error) {
	start := 0
	if integerLag < 0 {
		start = -integerLag
	}
	end := len(ref)
	if len(dst)-integerLag < end {
		end = len(dst) - integerLag
	}
	guard := int(0.06 * float64(rate))
	start += guard
	end -= guard
	if end-start < rate/8 {
		return transferResult{}, nil, fmt.Errorf("aligned overlap too short for spectral analysis")
	}
	a := ref[start:end]
	b := dst[start+integerLag : end+integerLag]

	n := opts.FFTSize
	if n <= 0 || n&(n-1) != 0 {
		return transferResult{}, nil, fmt.Errorf("FFT size must be a positive power of two")
	}
	var warnings []string
	if n > len(a) {
		n = 1
		for n*2 <= len(a) {
			n *= 2
		}
		warnings = append(warnings, fmt.Sprintf("FFT size reduced to %d samples because the aligned overlap is short", n))
	}
	if n < 4096 {
		return transferResult{}, warnings, fmt.Errorf("aligned overlap needs at least 4096 samples (have %d)", len(a))
	}
	plan, err := dsp.NewFFTPlan(n)
	if err != nil {
		return transferResult{}, warnings, err
	}
	window := dsp.Hann(n)
	hop := n / 2
	frameCount := (len(a)-n)/hop + 1
	if frameCount < 2 {
		warnings = append(warnings, "only one spectral frame was available; phase noise and boundary contamination are not separable")
	}
	stride := 1
	if frameCount > 256 {
		stride = (frameCount + 255) / 256
	}
	sxx := make([]float64, n/2+1)
	syy := make([]float64, n/2+1)
	sxy := make([]complex128, n/2+1)
	bufA := make([]complex128, n)
	bufB := make([]complex128, n)
	used := 0
	for frame := 0; frame < frameCount; frame += stride {
		offset := frame * hop
		var meanA, meanB float64
		for i := 0; i < n; i++ {
			meanA += a[offset+i]
			meanB += b[offset+i]
		}
		meanA /= float64(n)
		meanB /= float64(n)
		for i := 0; i < n; i++ {
			bufA[i] = complex((a[offset+i]-meanA)*window[i], 0)
			bufB[i] = complex((b[offset+i]-meanB)*window[i], 0)
		}
		if err := plan.Transform(bufA, false); err != nil {
			return transferResult{}, warnings, err
		}
		if err := plan.Transform(bufB, false); err != nil {
			return transferResult{}, warnings, err
		}
		for k := 0; k <= n/2; k++ {
			ar, br := bufA[k], bufB[k]
			sxx[k] += real(ar)*real(ar) + imag(ar)*imag(ar)
			syy[k] += real(br)*real(br) + imag(br)*imag(br)
			sxy[k] += cmplx.Conj(ar) * br
		}
		used++
	}
	h := make([]complex128, n/2+1)
	coh := make([]float64, n/2+1)
	maxPower := 0.0
	for _, v := range sxx {
		if v > maxPower {
			maxPower = v
		}
	}
	powerFloor := maxPower * 1e-6
	for k := range h {
		if sxx[k] > 0 {
			h[k] = sxy[k] / complex(sxx[k], 0)
		}
		den := sxx[k] * syy[k]
		if den > 0 {
			coh[k] = math.Pow(cmplx.Abs(sxy[k]), 2) / den
		}
	}

	df := float64(rate) / float64(n)
	valid := make([]int, 0, n/2)
	var phases, ks, weights []float64
	for k := 1; k <= n/2; k++ {
		f := float64(k) * df
		if f < 20 || f > opts.CutoffHz {
			continue
		}
		if sxx[k] < powerFloor || coh[k] < opts.MinCoherence {
			continue
		}
		valid = append(valid, k)
		ks = append(ks, float64(k))
		phases = append(phases, math.Atan2(imag(h[k]), real(h[k])))
		weights = append(weights, coh[k]*coh[k])
	}
	if len(valid) < 8 {
		return transferResult{}, warnings, fmt.Errorf("fewer than 8 coherent bins below %.0f Hz; cannot estimate phase", opts.CutoffHz)
	}
	dsp.UnwrapPhase(phases)
	slope, intercept := dsp.LinearFit(ks, phases, weights)
	var residual, weightSum float64
	for i := range phases {
		d := phases[i] - (intercept + slope*ks[i])
		residual += weights[i] * d * d
		weightSum += weights[i]
	}
	phaseRMS := math.NaN()
	if weightSum > 0 {
		phaseRMS = math.Sqrt(residual/weightSum) * 180 / math.Pi
	}
	var cohs []float64
	for _, k := range valid {
		cohs = append(cohs, coh[k])
	}
	var gdSamples []float64
	for i := 1; i < len(valid); i++ {
		dk := valid[i] - valid[i-1]
		if dk <= 0 || dk > 8 {
			continue
		}
		dphi := phases[i] - phases[i-1]
		gdSamples = append(gdSamples, -dphi/float64(dk)*float64(n)/(2*math.Pi))
	}
	ripple := math.NaN()
	if len(gdSamples) > 3 {
		ripple = (dsp.Quantile(gdSamples, 0.95) - dsp.Quantile(gdSamples, 0.05)) / float64(rate) * 1000
	}
	result := transferResult{
		h:           h,
		coherence:   coh,
		valid:       valid,
		phase:       phases,
		fftSize:     n,
		frames:      used,
		slope:       slope,
		intercept:   intercept,
		fitDelayMS:  -slope * float64(n) / (2 * math.Pi) / float64(rate) * 1000,
		phaseRMSDeg: phaseRMS,
		rippleMS:    ripple,
		medianCoh:   dsp.Median(cohs),
		reliable:    used >= 8 && dsp.Median(cohs) >= 0.9,
	}
	if !result.reliable {
		warnings = append(warnings, "low frame count or coherence: treat group-delay shape and impulse tail as qualitative")
	}
	return result, warnings, nil
}

func groupDelayBands(tr transferResult, rate, integerLag int, opts Options) GroupDelayReport {
	df := float64(rate) / float64(tr.fftSize)
	bands := []struct {
		name      string
		low, high float64
	}{
		{"low", 100, 1000},
		{"mid", 1000, 5000},
		{"presence", 5000, 10000},
		{"high", 10000, math.Min(16000, opts.CutoffHz)},
		{"air", math.Min(16000, opts.CutoffHz), opts.CutoffHz},
	}
	var out []BandDelay
	for _, band := range bands {
		if band.high <= band.low || band.high-band.low < 100 {
			continue
		}
		var ks, ps, ws []float64
		var bandCoh []float64
		for i, k := range tr.valid {
			f := float64(k) * df
			if f < band.low || f > band.high {
				continue
			}
			ks = append(ks, float64(k))
			ps = append(ps, tr.phase[i])
			ws = append(ws, tr.coherence[k]*tr.coherence[k])
			bandCoh = append(bandCoh, tr.coherence[k])
		}
		if len(ks) < 5 {
			continue
		}
		slope, intercept := dsp.LinearFit(ks, ps, ws)
		var gd []float64
		for i := 1; i < len(ks); i++ {
			dk := ks[i] - ks[i-1]
			if dk <= 0 || dk > 8 {
				continue
			}
			gd = append(gd, -((ps[i]-ps[i-1])/dk)*float64(tr.fftSize)/(2*math.Pi))
		}
		var rms, sw float64
		for i := range ks {
			d := ps[i] - (intercept + slope*ks[i])
			rms += ws[i] * d * d
			sw += ws[i]
		}
		if sw > 0 {
			rms = math.Sqrt(rms/sw) * 180 / math.Pi
		}
		gdm := -slope * float64(tr.fftSize) / (2 * math.Pi) / float64(rate) * 1000
		rip := math.NaN()
		if len(gd) > 3 {
			rip = (dsp.Quantile(gd, 0.95) - dsp.Quantile(gd, 0.05)) / float64(rate) * 1000
		}
		out = append(out, BandDelay{
			Name: band.name, LowHz: band.low, HighHz: band.high,
			GroupDelayMS: gdm,
			TotalDelayMS: float64(integerLag)/float64(rate)*1000 + gdm,
			RippleMS:     rip, PhaseRMSError: rms,
			Coherence: dsp.Median(bandCoh), Bins: len(ks),
		})
	}
	return GroupDelayReport{
		Bands: out, MedianDelayMS: tr.fitDelayMS, RippleMS: tr.rippleMS,
		PhaseLinearityRMSDeg: tr.phaseRMSDeg, ReferenceFitDelayMS: tr.fitDelayMS,
		CoherenceGate: opts.MinCoherence, ValidPassbandBins: len(tr.valid),
		EffectiveBandwidthHz: estimateBandwidth(tr, rate, opts.CutoffHz),
		PhaseCurveIsReliable: tr.reliable, AggregateSpectrumUsed: false,
	}
}

func estimateBandwidth(tr transferResult, rate int, maxHz float64) float64 {
	df := float64(rate) / float64(tr.fftSize)
	var ref []float64
	for k := 1; k <= tr.fftSize/2; k++ {
		f := float64(k) * df
		if f >= 200 && f <= 2000 {
			ref = append(ref, cmplx.Abs(tr.h[k]))
		}
	}
	if len(ref) < 8 {
		return df * float64(tr.fftSize/2)
	}
	refMag := dsp.Median(ref)
	if refMag <= 0 {
		return 0
	}
	smoothBins := maxInt(1, int(100/df))
	last := 0.0
	for k := 1; k <= tr.fftSize/2; k++ {
		f := float64(k) * df
		if f < 100 || f > maxHz {
			continue
		}
		lo := k - smoothBins
		hi := k + smoothBins
		if lo < 1 {
			lo = 1
		}
		if hi > tr.fftSize/2 {
			hi = tr.fftSize / 2
		}
		var power float64
		for j := lo; j <= hi; j++ {
			m := cmplx.Abs(tr.h[j])
			power += m * m
		}
		power /= float64(hi - lo + 1)
		if math.Sqrt(power) >= refMag*math.Sqrt(0.5) {
			last = f
		}
	}
	return last
}

package analysis

import (
	"math"
	"math/cmplx"

	"resampleforensics/internal/dsp"
)

func impulseMetrics(tr transferResult, rate, integerLag int) ImpulseReport {
	n := tr.fftSize
	half := n / 2
	spec := make([]complex128, n)
	for k := 0; k <= half; k++ {
		rot := cmplx.Rect(1, tr.slope*float64(k))
		v := tr.h[k] * rot
		if k == 0 || k == half {
			v = complex(real(v), 0)
		}
		spec[k] = v
		if k > 0 && k < half {
			spec[n-k] = cmplx.Conj(v)
		}
	}
	plan, err := dsp.NewFFTPlan(n)
	if err != nil {
		return ImpulseReport{}
	}
	if err := plan.Transform(spec, true); err != nil {
		return ImpulseReport{}
	}
	ir := make([]float64, n)
	for i := range ir {
		ir[i] = real(spec[i])
	}

	maxSpan := int(math.Min(float64(n/4), 0.10*float64(rate)))
	if maxSpan < 8 {
		maxSpan = 8
	}
	peakT, peakV := 0, 0.0
	for t := -maxSpan; t <= maxSpan; t++ {
		v := math.Abs(irAt(ir, t))
		if v > peakV {
			peakT, peakV = t, v
		}
	}
	if peakV == 0 {
		return ImpulseReport{Symmetry: math.NaN(), PreEnergyDB: math.NaN()}
	}
	rotation := 1.0
	if irAt(ir, peakT) < 0 {
		rotation = -1
	}

	w := int(0.00015 * float64(rate))
	if w < 1 {
		w = 1
	}
	env := make([]float64, 2*maxSpan+1+w*2)
	envOffset := maxSpan + w
	for t := -maxSpan - w; t <= maxSpan+w; t++ {
		var sum float64
		for j := t - w; j <= t+w; j++ {
			v := rotation * irAt(ir, peakT+j)
			sum += v * v
		}
		env[t+envOffset] = math.Sqrt(sum / float64(2*w+1))
	}
	envAt := func(t int) float64 { return env[t+envOffset] }
	peakEnv := envAt(0)
	for t := -w; t <= w; t++ {
		if envAt(t) > peakEnv {
			peakEnv = envAt(t)
		}
	}
	if peakEnv <= 0 {
		peakEnv = math.Abs(rotation * irAt(ir, peakT))
	}
	outer := make([]float64, 0, 2*maxSpan/10+2)
	for t := -maxSpan; t < -maxSpan+maxSpan/10; t++ {
		outer = append(outer, envAt(t))
	}
	for t := maxSpan - maxSpan/10; t <= maxSpan; t++ {
		outer = append(outer, envAt(t))
	}
	noiseFloor := dsp.Median(outer)
	thr60 := math.Max(peakEnv*0.001, noiseFloor*2.5)
	thr40 := math.Max(peakEnv*0.01, noiseFloor*2.5)
	left60, right60 := contiguousSupport(envAt, thr60, maxSpan)
	left40, right40 := contiguousSupport(envAt, thr40, maxSpan)
	span40 := float64(right40-left40) / float64(rate) * 1000
	span60 := float64(right60-left60) / float64(rate) * 1000
	extent := maxInt(absInt(left60), absInt(right60))
	extent += extent/5 + 2
	if extent > maxSpan {
		extent = maxSpan
	}

	var pre, post, total float64
	for t := -extent; t <= extent; t++ {
		v := rotation * irAt(ir, peakT+t)
		e := math.Max(v*v-noiseFloor*noiseFloor, 0)
		total += e
		if t < 0 {
			pre += e
		} else if t > 0 {
			post += e
		}
	}
	preDB := 10 * math.Log10(math.Max(pre, 1e-30)/math.Max(post, 1e-30))
	e99 := energySpan99(ir, peakT, extent, total, noiseFloor, rotation)
	symSpan := maxInt(absInt(left60), absInt(right60))
	if symSpan < 8 {
		symSpan = 8
	}
	if maxSym := int(0.02 * float64(rate)); symSpan > maxSym {
		symSpan = maxSym
	}
	symmetry := irSymmetry(ir, peakT, symSpan, rotation)

	delaySamples := float64(integerLag) + tr.fitDelayMS/1000*float64(rate)
	linear := symmetry >= 0.80 && tr.reliable
	taps := math.NaN()
	note := "converter delay may be internally compensated; support is an effective length, not a unique design-tap count"
	if linear && math.Abs(delaySamples) >= 1 {
		taps = 2*math.Abs(delaySamples) + 1
		note = "linear-phase estimate assumes the observed latency was not compensated inside the converter"
	}
	return ImpulseReport{
		Support40DBMS: span40, Support60DBMS: span60,
		Energy99MS:       float64(e99) / float64(rate) * 1000,
		Support60Samples: float64(right60 - left60),
		PreEnergyDB:      preDB, Symmetry: symmetry,
		PeakResidualMS:     float64(peakT) / float64(rate) * 1000,
		LinearPhaseLikely:  linear,
		LatencyImpliedTaps: taps, TapsNote: note,
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func irAt(ir []float64, t int) float64 {
	n := len(ir)
	t %= n
	if t < 0 {
		t += n
	}
	return ir[t]
}

func contiguousSupport(envAt func(int) float64, threshold float64, maxSpan int) (int, int) {
	left, right := 0, 0
	for left > -maxSpan && envAt(left-1) >= threshold {
		left--
	}
	for right < maxSpan && envAt(right+1) >= threshold {
		right++
	}
	return left, right
}

func energySpan99(ir []float64, peakT, maxSpan int, total, noiseFloor, rotation float64) int {
	if total <= 0 {
		return 0
	}
	left, right := 0, 0
	energyAt := func(t int) float64 {
		v := rotation * irAt(ir, peakT+t)
		return math.Max(v*v-noiseFloor*noiseFloor, 0)
	}
	acc := energyAt(0)
	for acc < 0.99*total && (left > -maxSpan || right < maxSpan) {
		le, re := 0.0, 0.0
		if left > -maxSpan {
			le = energyAt(left - 1)
		}
		if right < maxSpan {
			re = energyAt(right + 1)
		}
		if le >= re && left > -maxSpan {
			left--
			acc += le
		} else if right < maxSpan {
			right++
			acc += re
		}
	}
	return right - left
}

func irSymmetry(ir []float64, peakT, span int, rotation float64) float64 {
	var sxy, sxx, syy float64
	for k := 1; k <= span; k++ {
		l := rotation * irAt(ir, peakT-k)
		r := rotation * irAt(ir, peakT+k)
		sxy += l * r
		sxx += l * l
		syy += r * r
	}
	if sxx <= 0 || syy <= 0 {
		return math.NaN()
	}
	return sxy / math.Sqrt(sxx*syy)
}

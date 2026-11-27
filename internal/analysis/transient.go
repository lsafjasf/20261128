package analysis

import (
	"math"
	"sort"

	"resampleforensics/internal/dsp"
)

type transientPoint struct {
	timeSec  float64
	strength float64
}

type envelopeMetrics struct {
	width10MS float64
	riseMS    float64
	lead50MS  float64
	tail50MS  float64
	leadPhase float64
	tailPhase float64
	prePostDB float64
	valid     bool
}

func detectTransients(source []float64, rate, maxCount int) []transientPoint {
	if len(source) < rate/5 || maxCount <= 0 {
		return nil
	}
	diff := make([]float64, len(source))
	for i := 1; i < len(source); i++ {
		d := source[i] - source[i-1]
		diff[i] = d * d
	}
	smooth := int(0.002 * float64(rate))
	if smooth < 1 {
		smooth = 1
	}
	det := movingAverage(diff, smooth)
	var vals []float64
	for _, v := range det {
		if v > 0 {
			vals = append(vals, v)
		}
	}
	if len(vals) < 20 {
		return nil
	}
	med := dsp.Median(vals)
	q95 := dsp.Quantile(vals, 0.95)
	threshold := math.Max(med*3, med+0.15*(q95-med))
	if threshold <= 0 {
		return nil
	}
	margin := int(0.08 * float64(rate))
	minGap := int(0.10 * float64(rate))
	type candidate struct {
		i int
		v float64
	}
	var candidates []candidate
	for i := margin + 1; i < len(det)-margin-1; i++ {
		if det[i] < threshold || det[i] < det[i-1] || det[i] < det[i+1] {
			continue
		}
		candidates = append(candidates, candidate{i: i, v: det[i]})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].v > candidates[j].v })
	var chosen []transientPoint
	for _, c := range candidates {
		ok := true
		for _, p := range chosen {
			if int(math.Abs(float64(c.i)-p.timeSec*float64(rate))) < minGap {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		chosen = append(chosen, transientPoint{timeSec: float64(c.i) / float64(rate), strength: c.v})
		if len(chosen) >= maxCount {
			break
		}
	}
	sort.Slice(chosen, func(i, j int) bool { return chosen[i].timeSec < chosen[j].timeSec })
	return chosen
}

func movingAverage(x []float64, width int) []float64 {
	if width < 1 {
		width = 1
	}
	out := make([]float64, len(x))
	var sum float64
	for i := range x {
		sum += x[i]
		if i >= width {
			sum -= x[i-width]
		}
		den := width
		if i+1 < width {
			den = i + 1
		}
		out[i] = sum / float64(den)
	}
	return out
}

func measureTransient(x []float64, rate int, centerIndex int) envelopeMetrics {
	pre := int(0.05 * float64(rate))
	post := int(0.15 * float64(rate))
	start := centerIndex - pre
	end := centerIndex + post
	if start < 0 || end > len(x) || end-start < rate/100 {
		return envelopeMetrics{}
	}
	w := append([]float64(nil), x[start:end]...)
	band := bandpass(w, rate, 1000, math.Min(8000, 0.45*float64(rate)))
	analytic, err := dsp.Hilbert(band)
	if err != nil {
		return envelopeMetrics{}
	}
	env := make([]float64, len(analytic))
	phase := make([]float64, len(analytic))
	for i, z := range analytic {
		env[i] = math.Hypot(real(z), imag(z))
		phase[i] = math.Atan2(imag(z), real(z))
	}
	env = movingAverage(env, maxInt(1, int(0.001*float64(rate))))
	center := pre
	searchLo := center - int(0.010*float64(rate))
	searchHi := center + int(0.040*float64(rate))
	if searchLo < 0 {
		searchLo = 0
	}
	if searchHi >= len(env) {
		searchHi = len(env) - 1
	}
	peak := searchLo
	for i := searchLo; i <= searchHi; i++ {
		if env[i] > env[peak] {
			peak = i
		}
	}
	baseEnd := maxInt(1, int(0.03*float64(rate)))
	baseline := dsp.Median(env[:minInt(baseEnd, peak)])
	maxv := env[peak]
	if maxv <= baseline*1.2 {
		return envelopeMetrics{}
	}
	thr10 := baseline + 0.10*(maxv-baseline)
	thr50 := baseline + 0.50*(maxv-baseline)
	thr90 := baseline + 0.90*(maxv-baseline)
	lead10 := crossingBackward(env, peak, thr10)
	tail10 := crossingForward(env, peak, thr10)
	lead50f := crossingFraction(env, peak, thr50, true)
	tail50f := crossingFraction(env, peak, thr50, false)
	lead90 := crossingFraction(env, peak, thr90, true)
	if lead10 < 0 || tail10 < 0 || math.IsNaN(lead50f) || math.IsNaN(tail50f) {
		return envelopeMetrics{}
	}
	preE, postE := windowEnergy(w, pre, int(0.010*float64(rate)), int(0.040*float64(rate)))
	db := 10 * math.Log10(math.Max(preE, 1e-30)/math.Max(postE, 1e-30))
	return envelopeMetrics{
		width10MS: float64(tail10-lead10) / float64(rate) * 1000,
		riseMS:    (lead90 - float64(lead10)) / float64(rate) * 1000,
		lead50MS:  float64(start+int(math.Round(lead50f)))/float64(rate)*1000 - float64(centerIndex)/float64(rate)*1000,
		tail50MS:  float64(start+int(math.Round(tail50f)))/float64(rate)*1000 - float64(centerIndex)/float64(rate)*1000,
		leadPhase: phaseAt(phase, lead50f),
		tailPhase: phaseAt(phase, tail50f),
		prePostDB: db,
		valid:     lead90 >= float64(lead10) && tail10 > peak,
	}
}

func crossingBackward(env []float64, peak int, threshold float64) int {
	for i := peak; i > 0; i-- {
		if env[i] < threshold {
			return i
		}
	}
	return -1
}

func crossingForward(env []float64, peak int, threshold float64) int {
	for i := peak; i+1 < len(env); i++ {
		if env[i] < threshold {
			return i
		}
	}
	return -1
}

func crossingFraction(env []float64, peak int, threshold float64, rise bool) float64 {
	if rise {
		for i := peak; i > 0; i-- {
			if env[i-1] < threshold && env[i] >= threshold {
				d := env[i] - env[i-1]
				if d == 0 {
					return float64(i)
				}
				return float64(i) - (env[i]-threshold)/d
			}
		}
		return math.NaN()
	}
	for i := peak; i+1 < len(env); i++ {
		if env[i] >= threshold && env[i+1] < threshold {
			d := env[i] - env[i+1]
			if d == 0 {
				return float64(i)
			}
			return float64(i) + (env[i]-threshold)/d
		}
	}
	return math.NaN()
}

func phaseAt(phase []float64, pos float64) float64 {
	i := int(math.Round(pos))
	if i < 0 {
		i = 0
	}
	if i >= len(phase) {
		i = len(phase) - 1
	}
	return phase[i]
}

func windowEnergy(x []float64, center, before, after int) (float64, float64) {
	var pre, post float64
	lo := center - before
	if lo < 0 {
		lo = 0
	}
	for i := lo; i < center && i < len(x); i++ {
		pre += x[i] * x[i]
	}
	hi := center + after
	if hi > len(x) {
		hi = len(x)
	}
	for i := center; i < hi; i++ {
		post += x[i] * x[i]
	}
	return pre, post
}

func bandpass(x []float64, rate int, low, high float64) []float64 {
	n := dsp.NextPowerOfTwo(len(x))
	plan, err := dsp.NewFFTPlan(n)
	if err != nil {
		return append([]float64(nil), x...)
	}
	buf := make([]complex128, n)
	for i, v := range x {
		buf[i] = complex(v, 0)
	}
	if err := plan.Transform(buf, false); err != nil {
		return append([]float64(nil), x...)
	}
	df := float64(rate) / float64(n)
	for k := 1; k <= n/2; k++ {
		f := float64(k) * df
		if f < low || f > high {
			buf[k] = 0
			buf[n-k] = 0
		}
	}
	buf[0] = 0
	if n%2 == 0 {
		buf[n/2] = 0
	}
	if err := plan.Transform(buf, true); err != nil {
		return append([]float64(nil), x...)
	}
	out := make([]float64, len(x))
	for i := range out {
		out[i] = real(buf[i])
	}
	return out
}

func wrapDegrees(v float64) float64 {
	for v > 180 {
		v -= 360
	}
	for v < -180 {
		v += 360
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

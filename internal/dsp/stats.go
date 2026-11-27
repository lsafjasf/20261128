package dsp

import (
	"math"
	"sort"
)

// Quantile returns the linearly interpolated p-quantile (p in [0,1]).
func Quantile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	if p <= 0 {
		p = 0
	} else if p >= 1 {
		p = 1
	}
	work := append([]float64(nil), values...)
	sort.Float64s(work)
	pos := p * float64(len(work)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return work[lo]
	}
	t := pos - float64(lo)
	return work[lo]*(1-t) + work[hi]*t
}

func Median(values []float64) float64 { return Quantile(values, 0.5) }

func RMS(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	var sum float64
	for _, v := range values {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(values)))
}

// LinearFit returns slope and intercept for y = slope*x + intercept using
// optional non-negative weights.
func LinearFit(x, y, weight []float64) (slope, intercept float64) {
	var sw, sx, sy, sxx, sxy float64
	for i := range x {
		w := 1.0
		if weight != nil {
			w = weight[i]
		}
		if w <= 0 {
			continue
		}
		sw += w
		sx += w * x[i]
		sy += w * y[i]
		sxx += w * x[i] * x[i]
		sxy += w * x[i] * y[i]
	}
	if sw == 0 {
		return math.NaN(), math.NaN()
	}
	den := sw*sxx - sx*sx
	if math.Abs(den) < 1e-15 {
		return 0, sy / sw
	}
	slope = (sw*sxy - sx*sy) / den
	intercept = (sy - slope*sx) / sw
	return slope, intercept
}

func UnwrapPhase(phase []float64) {
	if len(phase) < 2 {
		return
	}
	correction := 0.0
	prevRaw := phase[0]
	for i := 1; i < len(phase); i++ {
		raw := phase[i]
		d := raw - prevRaw
		for d > math.Pi {
			correction -= 2 * math.Pi
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			correction += 2 * math.Pi
			d += 2 * math.Pi
		}
		phase[i] = raw + correction
		prevRaw = raw
	}
}

func Clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

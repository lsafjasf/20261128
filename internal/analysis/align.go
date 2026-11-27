package analysis

import (
	"math"

	"resampleforensics/internal/dsp"
)

// Alignment is the observed destination latency. Positive LagSamples means
// dst[i+LagSamples] corresponds to ref[i].
type Alignment struct {
	LagSamples  float64
	IntegerLag  int
	Confidence  float64
	SearchWidth int
}

func Align(ref, dst []float64, rate int) Alignment {
	if len(ref) < rate/20 || len(dst) < rate/20 {
		return Alignment{}
	}
	decim := rate / 1500
	if decim < 1 {
		decim = 1
	}
	refEnv := smoothAbsDecimate(ref, rate/200, decim)
	dstEnv := smoothAbsDecimate(dst, rate/200, decim)
	maxLag := int(math.Round(0.35 * float64(rate) / float64(decim)))
	lag, conf := bestLag(refEnv, dstEnv, maxLag)
	coarse := lag * float64(decim)

	// Refine on the actual waveform around the strongest reference transient.
	peak := strongestPeak(refEnv, maxLag)
	if peak >= 0 {
		center := peak * decim
		radius := int(0.012 * float64(rate))
		search := int(0.010 * float64(rate))
		lag, conf = localLag(ref, dst, center, radius, int(math.Round(coarse)), search)
	} else {
		lag = coarse
	}
	return Alignment{
		LagSamples:  lag,
		IntegerLag:  int(math.Round(lag)),
		Confidence:  conf,
		SearchWidth: maxLag * decim,
	}
}

func smoothAbsDecimate(x []float64, smooth, decim int) []float64 {
	if decim < 1 {
		decim = 1
	}
	if smooth < 1 {
		smooth = 1
	}
	out := make([]float64, (len(x)+decim-1)/decim)
	var running float64
	for i := range x {
		running += math.Abs(x[i])
		if i >= smooth {
			running -= math.Abs(x[i-smooth])
		}
		if i%decim == 0 {
			den := smooth
			if i+1 < smooth {
				den = i + 1
			}
			out[i/decim] = running / float64(den)
		}
	}
	return out
}

func bestLag(a, b []float64, maxLag int) (float64, float64) {
	if maxLag < 1 {
		maxLag = 1
	}
	if maxLag > len(a)-8 {
		maxLag = len(a) - 8
	}
	if maxLag > len(b)-8 {
		maxLag = len(b) - 8
	}
	if maxLag < 1 {
		return 0, 0
	}
	best := math.Inf(-1)
	bestLag := 0.0
	corrs := make([]float64, 2*maxLag+1)
	for lag := -maxLag; lag <= maxLag; lag++ {
		startA, endA := 0, len(a)
		startB := lag
		if startB < 0 {
			startA = -startB
			startB = 0
		}
		if endA > len(a) {
			endA = len(a)
		}
		if endB := startB + (endA - startA); endB < len(b) {
			endA = startA + (endB - startB)
		} else {
			endA = startA + (len(b) - startB)
		}
		c, ok := normalizedCorr(a[startA:endA], b[startB:startB+endA-startA])
		if !ok {
			continue
		}
		corrs[lag+maxLag] = c
		if c > best {
			best = c
			bestLag = float64(lag)
		}
	}
	idx := int(bestLag) + maxLag
	if idx > 0 && idx+1 < len(corrs) {
		y0, y1, y2 := corrs[idx-1], corrs[idx], corrs[idx+1]
		den := y0 - 2*y1 + y2
		if math.Abs(den) > 1e-12 {
			bestLag += 0.5 * (y0 - y2) / den
		}
	}
	return bestLag, math.Max(0, best)
}

func normalizedCorr(a, b []float64) (float64, bool) {
	if len(a) < 8 || len(a) != len(b) {
		return 0, false
	}
	var ma, mb float64
	for i := range a {
		ma += a[i]
		mb += b[i]
	}
	ma /= float64(len(a))
	mb /= float64(len(b))
	var ab, aa, bb float64
	for i := range a {
		x := a[i] - ma
		y := b[i] - mb
		ab += x * y
		aa += x * x
		bb += y * y
	}
	if aa < 1e-15 || bb < 1e-15 {
		return 0, false
	}
	return ab / math.Sqrt(aa*bb), true
}

func strongestPeak(env []float64, margin int) int {
	lo, hi := margin, len(env)-margin
	if lo < 1 {
		lo = 1
	}
	if hi > len(env)-2 {
		hi = len(env) - 2
	}
	best, bestI := math.Inf(-1), -1
	for i := lo; i < hi; i++ {
		if env[i] >= env[i-1] && env[i] >= env[i+1] && env[i] > best {
			best, bestI = env[i], i
		}
	}
	return bestI
}

func localLag(ref, dst []float64, center, radius, coarse, search int) (float64, float64) {
	start := center - radius
	end := center + radius
	if start < 0 {
		start = 0
	}
	if end > len(ref) {
		end = len(ref)
	}
	if end-start < 16 {
		return float64(coarse), 0
	}
	corrs := make([]float64, 2*search+1)
	best := math.Inf(-1)
	bestLag := coarse
	for lag := coarse - search; lag <= coarse+search; lag++ {
		bs := start + lag
		be := end + lag
		if bs < 0 || be > len(dst) {
			continue
		}
		c, ok := normalizedCorr(ref[start:end], dst[bs:be])
		if !ok {
			continue
		}
		corrs[lag-coarse+search] = c
		if c > best {
			best = c
			bestLag = lag
		}
	}
	idx := bestLag - coarse + search
	if idx > 0 && idx+1 < len(corrs) {
		y0, y1, y2 := corrs[idx-1], corrs[idx], corrs[idx+1]
		den := y0 - 2*y1 + y2
		if math.Abs(den) > 1e-12 {
			bestLagF := float64(bestLag) + 0.5*(y0-y2)/den
			return bestLagF, math.Max(0, best)
		}
	}
	return float64(bestLag), math.Max(0, best)
}

func alignedIndex(i, lag int) int { return i + lag }

func rmsRange(x []float64, lo, hi int) float64 {
	if lo < 0 {
		lo = 0
	}
	if hi > len(x) {
		hi = len(x)
	}
	if hi <= lo {
		return 0
	}
	return dsp.RMS(x[lo:hi])
}

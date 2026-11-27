package audio

import "math"

// EstimatePitchHz estimates the dominant periodic frequency of a signal
// via normalized autocorrelation on its central span. It reads the
// rendered waveform directly rather than trusting metadata, so a naive
// resampler cannot hide a pitch shift behind stale source metadata. A
// zero-crossing fallback covers degenerate short buffers.
func EstimatePitchHz(x []float64, sampleRateHz int) (float64, bool) {
	if len(x) < 4 {
		return 0, false
	}

	if f, ok := autocorrelationPitch(x, sampleRateHz); ok {
		return f, true
	}

	return zeroCrossingPitch(x, sampleRateHz)
}

func autocorrelationPitch(x []float64, sampleRateHz int) (float64, bool) {
	// Trim 10% at each end so TSM crossfade ramps do not bias the estimate.
	lo := len(x) / 10
	hi := len(x) - lo
	if hi-lo < sampleRateHz/4 {
		lo, hi = 0, len(x)
	}
	y := x[lo:hi]

	minLag := 2                 // skip the trivial lag-0/1 self correlation
	maxLag := sampleRateHz / 50 // floor at 50 Hz
	if maxLag >= len(y) {
		maxLag = len(y) - 1
	}

	corrs := make([]float64, maxLag+1)
	globalMax := 0.0
	for lag := minLag; lag <= maxLag; lag++ {
		var num, d0, d1 float64
		for i := 0; i+lag < len(y); i++ {
			num += y[i] * y[i+lag]
			d0 += y[i] * y[i]
			d1 += y[i+lag] * y[i+lag]
		}
		denom := math.Sqrt(d0 * d1)
		if denom == 0 {
			continue
		}
		corr := num / denom
		corrs[lag] = corr
		if corr > globalMax {
			globalMax = corr
		}
	}
	if globalMax < 0.5 {
		return 0, false
	}

	// Pick the first local maximum that is a strong relative peak. This
	// resolves the fundamental period instead of a higher-frequency
	// correlation peak that happens to score slightly higher.
	threshold := 0.7 * globalMax
	for lag := minLag + 1; lag < maxLag; lag++ {
		if corrs[lag] >= threshold &&
			corrs[lag] >= corrs[lag-1] && corrs[lag] >= corrs[lag+1] {
			return float64(sampleRateHz) / float64(lag), true
		}
	}
	return 0, false
}

func zeroCrossingPitch(x []float64, sampleRateHz int) (float64, bool) {
	crossings := 0
	prev := x[0]
	for i := 1; i < len(x); i++ {
		if (prev <= 0 && x[i] > 0) || (prev > 0 && x[i] <= 0) {
			crossings++
		}
		prev = x[i]
	}
	if crossings < 2 {
		return 0, false
	}
	duration := float64(len(x)-1) / float64(sampleRateHz)
	// Two zero crossings per period.
	return float64(crossings) / (2 * duration), true
}

// RMS returns the root-mean-square amplitude, used to confirm TSM
// normalization preserves level across crossfades.
func RMS(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var sum float64
	for _, v := range x {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(x)))
}

// GenerateSine renders a sine wave at freqHz, optionally fading in and
// out so framing edges do not dominate the pitch estimate.
func GenerateSine(freqHz, durationSec float64, sampleRateHz int) []float64 {
	n := int(math.Round(durationSec * float64(sampleRateHz)))
	out := make([]float64, n)
	fade := n / 20
	for i := range out {
		v := math.Sin(2 * math.Pi * freqHz * float64(i) / float64(sampleRateHz))
		if fade > 0 {
			if i < fade {
				v *= float64(i) / float64(fade)
			} else if i > n-fade {
				v *= float64(n-i) / float64(fade)
			}
		}
		out[i] = v
	}
	return out
}

package synth

import "math"

func KickBass(sampleRate int, seconds float64) []float64 {
	samples := make([]float64, int(float64(sampleRate)*seconds))
	kicks := []float64{0.20, 0.80}
	twoPi := 2 * math.Pi

	for i := range samples {
		timeSeconds := float64(i) / float64(sampleRate)
		bass := 0.30 * math.Sin(twoPi*55*timeSeconds)
		air := 0.035 * math.Sin(twoPi*4200*timeSeconds)
		sample := bass + air
		for _, kickTime := range kicks {
			delta := timeSeconds - kickTime
			if delta >= 0 {
				phase := twoPi * (90*delta - 55*delta*(1-math.Exp(-22*delta)))
				envelope := math.Exp(-13 * delta)
				click := 0.18 * math.Exp(-180*delta) * math.Cos(twoPi*1800*delta)
				sample += 0.72*envelope*math.Sin(phase) + click
			}
		}
		samples[i] = sample
	}

	normalize(samples, math.Pow(10, -0.5/20))
	return samples
}

func normalize(samples []float64, targetPeak float64) {
	peak := 0.0
	for _, sample := range samples {
		amplitude := math.Abs(sample)
		if amplitude > peak {
			peak = amplitude
		}
	}
	gain := targetPeak / peak
	for i := range samples {
		samples[i] *= gain
	}
}

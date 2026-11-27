package lfcheck

import "math"

type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
	x1, x2     float64
	y1, y2     float64
}

func (f *biquad) process(x float64) float64 {
	y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	f.x2, f.x1 = f.x1, x
	f.y2, f.y1 = f.y1, y
	return y
}

func lowPassStage(cutoff, sampleRate, q float64) biquad {
	w := 2 * math.Pi * cutoff / sampleRate
	cosW, sinW := math.Cos(w), math.Sin(w)
	alpha := sinW / (2 * q)
	a0 := 1 + alpha
	return biquad{
		b0: ((1 - cosW) / 2) / a0,
		b1: (1 - cosW) / a0,
		b2: ((1 - cosW) / 2) / a0,
		a1: (-2 * cosW) / a0,
		a2: (1 - alpha) / a0,
	}
}

func highPassStage(cutoff, sampleRate, q float64) biquad {
	w := 2 * math.Pi * cutoff / sampleRate
	cosW, sinW := math.Cos(w), math.Sin(w)
	alpha := sinW / (2 * q)
	a0 := 1 + alpha
	return biquad{
		b0: ((1 + cosW) / 2) / a0,
		b1: (-(1 + cosW)) / a0,
		b2: ((1 + cosW) / 2) / a0,
		a1: (-2 * cosW) / a0,
		a2: (1 - alpha) / a0,
	}
}

// lowBand applies a fourth-order Butterworth band limit. The LFE channel is
// specified as 20-120 Hz by cinema practice; the same band is applied to all
// channels so relative delay estimates are not biased by filtering.
func lowBand(samples []float64, sampleRate int, low, high float64) []float64 {
	if sampleRate <= 0 || len(samples) == 0 {
		return nil
	}
	if high >= float64(sampleRate)/2 {
		high = 0.45 * float64(sampleRate)
	}
	out := append([]float64(nil), samples...)
	stages := []biquad{
		highPassStage(low, float64(sampleRate), 0.541196100146197),
		highPassStage(low, float64(sampleRate), 1.306562964876377),
		lowPassStage(high, float64(sampleRate), 0.541196100146197),
		lowPassStage(high, float64(sampleRate), 1.306562964876377),
	}
	for i := range out {
		for s := range stages {
			out[i] = stages[s].process(out[i])
		}
	}
	return out
}

func rms(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, value := range samples {
		sum += value * value
	}
	return math.Sqrt(sum / float64(len(samples)))
}

func dbfs(value float64) float64 {
	if value <= 0 {
		return -180
	}
	return 20 * math.Log10(value)
}

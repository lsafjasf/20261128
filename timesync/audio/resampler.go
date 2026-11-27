// Package audio separates audio resampling from time remapping.
//
// The remap curve only answers "which source time does output frame n
// render?". A Resampler decides how source samples are rearranged into
// those frames:
//
//   - LinearResampler pulls interpolated samples at mapped positions. It
//     follows the curve exactly but rescales the waveform, so pitch moves
//     with the rate (chipmunk/baritone artifacts). This models the bug.
//   - TSMResampler applies time-scale modification: fixed synthesis hop
//     with variable analysis hop driven locally by the curve. Output frame
//     positions still match the remap timeline while spectral content and
//     therefore pitch are preserved. This is the corrected path.
package audio

import "timesync/remap"

// Resampler renders output samples for a remapping plan.
type Resampler interface {
	Name() string
	Resample(input []float64, plan remap.SamplePlan) []float64
}

// LinearResampler performs per-frame linear interpolation at the source
// position requested by the remap curve. No global offset is used.
type LinearResampler struct{}

// Name implements Resampler.
func (LinearResampler) Name() string { return "linear-resample" }

// Resample implements Resampler.
func (LinearResampler) Resample(input []float64, plan remap.SamplePlan) []float64 {
	outN := plan.OutputFrameCount()
	out := make([]float64, outN)
	if len(input) < 2 {
		return out
	}
	srcRate := float64(plan.SourceRateHz)
	for n := 0; n < outN; n++ {
		pos := plan.SourceTimeOf(n) * srcRate
		i := int(pos)
		frac := pos - float64(i)
		if i < 0 {
			out[n] = input[0]
			continue
		}
		if i >= len(input)-1 {
			out[n] = input[len(input)-1]
			continue
		}
		out[n] = input[i]*(1-frac) + input[i+1]*frac
	}
	return out
}

package remap

import "math"

// SamplePlan binds the continuous time curve to discrete sample indices.
type SamplePlan struct {
	Curve            *Curve
	SourceRateHz     int
	OutputRateHz     int
	SourceFrameCount int
}

// OutputFrameCount is the number of output samples needed to cover the
// remapped timeline at the output sample rate.
func (p SamplePlan) OutputFrameCount() int {
	return int(math.Ceil(p.Curve.OutputDuration()*float64(p.OutputRateHz) - 1e-9))
}

// OutputTimeOf maps an output frame index to its output timeline time.
func (p SamplePlan) OutputTimeOf(outFrame int) float64 {
	return float64(outFrame) / float64(p.OutputRateHz)
}

// SourceTimeOf maps an output frame to the exact source time it must
// render, inverting the remap curve locally for every frame.
func (p SamplePlan) SourceTimeOf(outFrame int) float64 {
	return p.Curve.Inverse(p.OutputTimeOf(outFrame))
}

// SourceFrameOf returns the nearest valid source frame index for an
// output frame. It is derived per frame from the curve, never from a
// shared offset, and is clamped to the available source range.
func (p SamplePlan) SourceFrameOf(outFrame int) int {
	idx := int(math.Round(p.SourceTimeOf(outFrame) * float64(p.SourceRateHz)))
	if idx < 0 {
		return 0
	}
	if idx >= p.SourceFrameCount {
		return p.SourceFrameCount - 1
	}
	return idx
}

// SourceFrameAt maps a source time to a source frame index.
func (p SamplePlan) SourceFrameAt(sourceTime float64) int {
	idx := int(math.Round(sourceTime * float64(p.SourceRateHz)))
	if idx < 0 {
		return 0
	}
	if idx >= p.SourceFrameCount {
		return p.SourceFrameCount - 1
	}
	return idx
}

// OutputFrameAt maps an output timeline time to an output frame index.
func (p SamplePlan) OutputFrameAt(outputTime float64) int {
	idx := int(math.Round(outputTime * float64(p.OutputRateHz)))
	max := p.OutputFrameCount()
	if idx < 0 {
		return 0
	}
	if idx >= max {
		return max - 1
	}
	return idx
}

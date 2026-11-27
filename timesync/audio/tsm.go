package audio

import (
	"math"
	"timesync/remap"
)

// DefaultTSMFrameLength is the analysis/synthesis frame length in output
// samples. 2048 at typical voice rates (~46-128 ms) gives smooth
// crossfades while keeping phoneme-level timing local.
const DefaultTSMFrameLength = 2048

// TSMResampler applies overlap-add time-scale modification.
//
// Synthesis frames are placed at a fixed hop, so output timing is a
// regular grid aligned with the remap timeline. Each analysis frame is
// fetched from the source position obtained by locally inverting the
// remap curve; the analysis hop therefore varies from frame to frame
// with the local rate. Samples inside a frame are never rescaled, which
// keeps the waveform's periodicity and hence its pitch intact.
type TSMResampler struct {
	FrameLength  int
	SynthesisHop int
}

// NewTSMResampler builds a TSM resampler with standard 75% overlap.
func NewTSMResampler() *TSMResampler {
	n := DefaultTSMFrameLength
	return &TSMResampler{FrameLength: n, SynthesisHop: n / 4}
}

// Name implements Resampler.
func (r *TSMResampler) Name() string { return "pitch-preserving-tsm" }

// Resample implements Resampler.
func (r *TSMResampler) Resample(input []float64, plan remap.SamplePlan) []float64 {
	n := r.FrameLength
	hop := r.SynthesisHop
	if n <= 0 || hop <= 0 || hop > n {
		panic("audio: invalid TSM frame parameters")
	}
	outN := plan.OutputFrameCount()
	out := make([]float64, outN)
	weight := make([]float64, outN)
	window := hannWindow(n)
	srcRate := float64(plan.SourceRateHz)
	half := n / 2

	// Synthesis centers start half a frame in so the first frame covers
	// output sample zero. Every center is mapped back to the source
	// independently through the curve.
	for center := half; center-half < outN; center += hop {
		sourcePos := plan.SourceTimeOf(center) * srcRate
		srcCenter := int(math.Round(sourcePos))
		for k := 0; k < n; k++ {
			outIdx := center - half + k
			if outIdx < 0 || outIdx >= outN {
				continue
			}
			srcIdx := srcCenter - half + k
			var sample float64
			if srcIdx >= 0 && srcIdx < len(input) {
				sample = input[srcIdx]
			}
			w := window[k]
			out[outIdx] += sample * w
			weight[outIdx] += w
		}
	}

	for i := range out {
		if weight[i] > 1e-9 {
			out[i] /= weight[i]
		}
	}
	return out
}

func hannWindow(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
	}
	return w
}

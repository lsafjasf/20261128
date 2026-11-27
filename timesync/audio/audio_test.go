package audio

import (
	"math"
	"testing"

	"timesync/remap"
)

func testPlan(t *testing.T, rates []float64) (remap.SamplePlan, []float64) {
	t.Helper()
	const rate = 16000
	var segs []remap.Segment
	for i, r := range rates {
		segs = append(segs, remap.Segment{
			S0: float64(i), S1: float64(i + 1),
			RateStart: r, RateEnd: r,
		})
	}
	curve, err := remap.NewCurve(segs)
	if err != nil {
		t.Fatal(err)
	}
	src := GenerateSine(220, curve.SourceDuration(), rate)
	plan := remap.SamplePlan{
		Curve: curve, SourceRateHz: rate, OutputRateHz: rate,
		SourceFrameCount: len(src),
	}
	return plan, src
}

func TestBothResamplersMatchTimelineLength(t *testing.T) {
	plan, src := testPlan(t, []float64{1, 0.2, 0.5, 2, 4})
	lin := LinearResampler{}.Resample(src, plan)
	tsm := NewTSMResampler().Resample(src, plan)
	want := plan.OutputFrameCount()
	if len(lin) != want || len(tsm) != want {
		t.Fatalf("lengths linear=%d tsm=%d want=%d", len(lin), len(tsm), want)
	}
	if rms := RMS(tsm); rms < 0.3 || rms > 0.9 {
		t.Fatalf("TSM normalized RMS out of range: %g", rms)
	}
}

func TestPitchBehavior(t *testing.T) {
	plan, src := testPlan(t, []float64{0.5, 0.5})
	lin := LinearResampler{}.Resample(src, plan)
	tsm := NewTSMResampler().Resample(src, plan)

	srcPitch, ok := EstimatePitchHz(src, plan.SourceRateHz)
	if !ok {
		t.Fatal("source pitch not detected")
	}
	linPitch, _ := EstimatePitchHz(lin, plan.OutputRateHz)
	tsmPitch, _ := EstimatePitchHz(tsm, plan.OutputRateHz)

	if math.Abs(tsmPitch/srcPitch-1) > 0.05 {
		t.Fatalf("TSM pitch drifted: src %g out %g", srcPitch, tsmPitch)
	}
	// At 0.5x playback the naive path must halve the pitch; this is the
	// defect the separated model exists to eliminate.
	if math.Abs(linPitch/srcPitch-0.5) > 0.1 {
		t.Fatalf("linear resampler expected ~0.5x pitch, got ratio %g", linPitch/srcPitch)
	}
}

func TestTSMFrameParameterValidation(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on invalid TSM parameters")
		}
	}()
	plan, src := testPlan(t, []float64{1})
	(&TSMResampler{FrameLength: 10, SynthesisHop: 20}).Resample(src, plan)
}

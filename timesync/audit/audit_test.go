package audit

import (
	"testing"

	"timesync/audio"
	"timesync/remap"
	"timesync/subs"
)

func buildInputs(t *testing.T) Inputs {
	t.Helper()
	const hz = 16000
	curve, err := remap.NewCurve([]remap.Segment{
		{S0: 0, S1: 2, RateStart: 1, RateEnd: 1},
		{S0: 2, S1: 4, RateStart: 0.2, RateEnd: 0.2},
		{S0: 4, S1: 6, RateStart: 2, RateEnd: 2},
		{S0: 6, S1: 8, RateStart: 4, RateEnd: 0.5},
	})
	if err != nil {
		t.Fatal(err)
	}
	src := audio.GenerateSine(220, curve.SourceDuration(), hz)
	plan := remap.SamplePlan{
		Curve: curve, SourceRateHz: hz, OutputRateHz: hz,
		SourceFrameCount: len(src),
	}
	events := []subs.Event{
		{ID: "w1", Text: "a", Start: 0.5, End: 1.5},
		{ID: "w2", Text: "b", Start: 2.5, End: 3.5},
		{ID: "w3", Text: "c", Start: 4.5, End: 5.5},
		{ID: "w4", Text: "d", Start: 6.5, End: 7.5},
	}
	mapped, err := subs.Remap(events, curve)
	if err != nil {
		t.Fatal(err)
	}
	return Inputs{
		Plan:         plan,
		Source:       src,
		LinearOutput: audio.LinearResampler{}.Resample(src, plan),
		TSMOutput:    audio.NewTSMResampler().Resample(src, plan),
		SourceEvents: events,
		MappedEvents: mapped,
	}
}

func TestEndToEndAuditPasses(t *testing.T) {
	rep := Run(buildInputs(t))
	if !rep.Pass {
		for _, f := range rep.Findings {
			t.Errorf("[%s] %s: %s", f.Severity, f.Where, f.Message)
		}
	}
	if !rep.Offset.Rejected {
		t.Fatal("audit failed to reject global-offset patching")
	}
	if !rep.SubtitleOK || !rep.Pitch.TSMPitchPreserved {
		t.Fatalf("subtitleOK=%v pitchPreserved=%v", rep.SubtitleOK, rep.Pitch.TSMPitchPreserved)
	}
	for i, seg := range rep.Segments {
		if !seg.LocalRateCheck {
			t.Errorf("segment %d local check failed", i)
		}
	}
}

func TestGlobalOffsetRejectedForPiecewiseCurve(t *testing.T) {
	in := buildInputs(t)
	check := rejectGlobalOffset(in.Plan.Curve)
	if !check.Rejected || check.Residual < 0.1 {
		t.Fatalf("offset check rejected=%v residual=%g", check.Rejected, check.Residual)
	}
}

func TestTamperedSubtitlesCaught(t *testing.T) {
	in := buildInputs(t)
	tampered := make([]subs.MappedEvent, len(in.MappedEvents))
	copy(tampered, in.MappedEvents)
	// Simulate the forbidden fix: shift every cue by one constant offset.
	for i := range tampered {
		tampered[i].StartOut += 0.25
		tampered[i].EndOut += 0.25
	}
	in.MappedEvents = tampered
	rep := Run(in)
	if rep.SubtitleOK || rep.Pass {
		t.Fatal("constant-offset subtitle patch was not detected")
	}
}

func TestTruncatedOutputCaught(t *testing.T) {
	in := buildInputs(t)
	in.TSMOutput = in.TSMOutput[:len(in.TSMOutput)/2]
	rep := Run(in)
	if rep.Pass {
		t.Fatal("truncated render unexpectedly passed")
	}
}

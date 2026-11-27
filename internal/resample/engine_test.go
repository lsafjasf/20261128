package resample

import (
	"math"
	"testing"
)

func baseCfg(mode string, missing map[int]bool) Config {
	return Config{
		SourceRate:   Rate{24, 1},
		OutputRate:   Rate{60, 1},
		SourceFrames: 48,
		Missing:      missing,
		Strategy:     Strategy{Mode: mode, Interpolation: "linear", MissingPolicy: "hold"},
	}
}

func TestTimelineTicks(t *testing.T) {
	tl := baseCfg("repeat", nil).timeline()
	if tl.ticksPerSec != 120 || tl.srcPeriod != 5 || tl.outPeriod != 2 {
		t.Fatalf("unexpected timeline: %+v", tl)
	}
}

func TestRepeatCadence(t *testing.T) {
	rep := Run(baseCfg("repeat", nil))
	if len(rep.Frames) != 120 {
		t.Fatalf("output frames = %d, want 120", len(rep.Frames))
	}
	// First five output slots: exact frame 0, two duplicates, then source 1.
	wantKind := []string{
		KindExact, KindRepeat, KindRepeat, KindRepeat, KindRepeat, KindExact,
	}
	for i, want := range wantKind {
		if rep.Frames[i].Kind != want {
			t.Fatalf("frame %d kind = %q, want %q", i, rep.Frames[i].Kind, want)
		}
	}
	if rep.Frames[1].EffectivePtsTicks != 0 || rep.Frames[3].EffectivePtsTicks != 5 {
		t.Fatalf("duplicate mapping wrong: %+v %+v", rep.Frames[1], rep.Frames[3])
	}
	if rep.Summary.ExactFrames != 24 || rep.Summary.DuplicateFrames != 96 {
		t.Fatalf("frame accounting wrong: %+v", rep.Summary)
	}
	if rep.Summary.RepeatPauseFrames != 72 || rep.Summary.JudderFrames != 119 {
		t.Fatalf("judder accounting wrong: %+v", rep.Summary)
	}
	if math.Abs(rep.Summary.RMSNormalizedJudder-1.2221691695471608) > 1e-12 {
		t.Fatalf("RMS = %v", rep.Summary.RMSNormalizedJudder)
	}
	if rep.Summary.PeakNormalizedJudder != 1.5 {
		t.Fatalf("peak = %v, want 1.5", rep.Summary.PeakNormalizedJudder)
	}
}

func TestBlendCadence(t *testing.T) {
	rep := Run(baseCfg("blend", nil))
	f1 := rep.Frames[1]
	if f1.Kind != KindSynthesized || !f1.Synthesized {
		t.Fatalf("frame 1 should be synthesized: %+v", f1)
	}
	if len(f1.SourceIndices) != 2 || f1.SourceIndices[0] != 0 || f1.SourceIndices[1] != 1 {
		t.Fatalf("frame 1 sources wrong: %+v", f1.SourceIndices)
	}
	if math.Abs(f1.Alpha-0.4) > 1e-9 {
		t.Fatalf("frame 1 alpha = %v, want 0.4", f1.Alpha)
	}
	// Exact source hits every fifth output frame.
	for i := 0; i < 24; i++ {
		if rep.Frames[i*5].Kind != KindExact {
			t.Fatalf("frame %d should be exact", i*5)
		}
	}
	// Two tail slots beyond the last source frame.
	if rep.Frames[118].Kind != KindTailHold || rep.Frames[119].Kind != KindTailHold {
		t.Fatalf("tail hold wrong: %q %q", rep.Frames[118].Kind, rep.Frames[119].Kind)
	}
	if rep.Summary.SynthesizedFrames != 94 || rep.Summary.SourceEndHoldFrames != 2 {
		t.Fatalf("blend accounting wrong: %+v", rep.Summary)
	}
	if rep.Summary.JudderFrames != 0 || rep.Summary.RMSNormalizedJudder != 0 {
		t.Fatalf("blend should be judder-free over source-covered span: %+v", rep.Summary)
	}
}

func TestGapHoldAndJump(t *testing.T) {
	missing := map[int]bool{12: true, 13: true}

	repeat := Run(baseCfg("repeat", missing))
	gap := repeat.Gaps
	if len(gap) != 1 || gap[0].MissingFrames != 2 ||
		gap[0].StartTicks != 60 || gap[0].EndTicks != 70 {
		t.Fatalf("gap record wrong: %+v", gap)
	}
	if repeat.Summary.MissingSourceFrames != 5 {
		t.Fatalf("repeat missing holds = %d, want 5", repeat.Summary.MissingSourceFrames)
	}

	blend := Run(baseCfg("blend", missing))
	if blend.Summary.GapHoldFrames != 7 || blend.Summary.MissingSourcePauseFrames != 7 {
		t.Fatalf("blend gap accounting wrong: %+v", blend.Summary)
	}
	// Frame 35 is the first real frame after the gap: one large catch-up jump.
	point := blend.JudderCurve[35]
	if point.Class != ClassMissingSourceJump || point.NormalizedError != 6.5 {
		t.Fatalf("catch-up jump wrong: %+v", point)
	}
	if blend.Summary.PeakNormalizedJudder != 6.5 {
		t.Fatalf("peak = %v, want 6.5", blend.Summary.PeakNormalizedJudder)
	}
	if math.Abs(blend.Summary.RMSNormalizedJudder-0.6438399758712677) > 1e-12 {
		t.Fatalf("blend gap RMS = %v", blend.Summary.RMSNormalizedJudder)
	}
}

func TestCompareDelta(t *testing.T) {
	before := Run(baseCfg("repeat", nil))
	after := Run(baseCfg("blend", nil))
	delta := Compare(before, after).Delta
	if delta.JudderFramesDelta != -119 || delta.RepeatPauseDelta != -72 {
		t.Fatalf("delta wrong: %+v", delta)
	}
	if delta.RMSNormalizedJudderDelta != -before.Summary.RMSNormalizedJudder {
		t.Fatalf("RMS delta wrong: %+v", delta)
	}
}

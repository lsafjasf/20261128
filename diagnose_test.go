package subburn

import (
	"testing"
	"time"
)

func standardCues() []Cue {
	var starts []time.Duration
	for at := 10 * time.Second; at <= 250*time.Second; at += 20 * time.Second {
		starts = append(starts, at)
	}
	return testCues(starts...)
}

func compareFor(t *testing.T, export RenderClock) []Comparison {
	t.Helper()
	rate := mustRate(t, "30000/1001")
	pipeline := Pipeline{
		Duration: 300 * time.Second,
		Preview:  ExactClock(rate),
		Export:   export,
	}
	comparisons, err := pipeline.CompareCues(standardCues())
	if err != nil {
		t.Fatalf("CompareCues: %v", err)
	}
	return comparisons
}

func TestDiagnoseFindsAccumulatedFrameTimeDrift(t *testing.T) {
	rate := mustRate(t, "30000/1001")
	comparisons := compareFor(t, RenderClock{
		Rate:      rate,
		FrameStep: 34 * time.Millisecond,
	})
	first := comparisons[0].Error
	last := comparisons[len(comparisons)-1].Error
	if first >= 0 || last >= first {
		t.Fatalf("expected progressively early cues, first=%s last=%s", first, last)
	}

	pipeline := Pipeline{
		Duration: 300 * time.Second,
		Preview:  ExactClock(rate),
		Export:   RenderClock{Rate: rate, FrameStep: 34 * time.Millisecond},
	}
	diagnosis, err := DiagnosePipeline(pipeline, standardCues())
	if err != nil {
		t.Fatalf("DiagnosePipeline: %v", err)
	}
	if diagnosis.Root != LayerFrameTime || !diagnosis.Cumulative() {
		t.Fatalf("root=%s cumulative=%v summary=%s findings=%+v",
			diagnosis.Root, diagnosis.Cumulative(), diagnosis.Summary, diagnosis.Findings)
	}
	if diagnosis.DriftPerMin > -100*time.Millisecond {
		t.Fatalf("per-minute drift = %s, want a visible early drift", diagnosis.DriftPerMin)
	}
}

func TestDiagnoseFindsRepeatedPrerollAsSteps(t *testing.T) {
	rate := mustRate(t, "25")
	export := RenderClock{
		Rate:           rate,
		Segment:        40 * time.Second,
		SegmentPreroll: 800 * time.Millisecond,
	}
	pipeline := Pipeline{
		Duration: 300 * time.Second,
		Preview:  ExactClock(rate),
		Export:   export,
	}
	diagnosis, err := DiagnosePipeline(pipeline, standardCues())
	if err != nil {
		t.Fatalf("DiagnosePipeline: %v", err)
	}
	if diagnosis.Root != LayerPreroll {
		t.Fatalf("root=%s summary=%s linearRMSE=%s stepRMSE=%s",
			diagnosis.Root, diagnosis.Summary, diagnosis.LinearRMSE, diagnosis.StepRMSE)
	}
	if diagnosis.StepRMSE > diagnosis.Quantization {
		t.Fatalf("step fit RMSE %s exceeds one frame %s",
			diagnosis.StepRMSE, diagnosis.Quantization)
	}
}

func TestConstantCueStartIsNotReportedAsAccumulation(t *testing.T) {
	rate := mustRate(t, "30000/1001")
	pipeline := Pipeline{
		Duration: 300 * time.Second,
		Preview:  ExactClock(rate),
		Export: RenderClock{
			Rate:           rate,
			SubtitleOrigin: 1500 * time.Millisecond,
		},
	}
	diagnosis, err := DiagnosePipeline(pipeline, standardCues())
	if err != nil {
		t.Fatalf("DiagnosePipeline: %v", err)
	}
	if diagnosis.Root != LayerOrigin || diagnosis.Cumulative() {
		t.Fatalf("root=%s cumulative=%v summary=%s",
			diagnosis.Root, diagnosis.Cumulative(), diagnosis.Summary)
	}
	if diagnosis.MaxAbsError > 1600*time.Millisecond ||
		diagnosis.MaxAbsError < 1400*time.Millisecond {
		t.Fatalf("constant error = %s", diagnosis.MaxAbsError)
	}
}

func TestOutputDisplayStartIsClassifiedSeparately(t *testing.T) {
	rate := mustRate(t, "25")
	pipeline := Pipeline{
		Duration: 300 * time.Second,
		Preview:  ExactClock(rate),
		Export: RenderClock{
			Rate:                rate,
			DisplayStart:        2 * time.Second,
			IncludeDisplayStart: true,
		},
	}
	diagnosis, err := DiagnosePipeline(pipeline, standardCues())
	if err != nil {
		t.Fatalf("DiagnosePipeline: %v", err)
	}
	if diagnosis.Root != LayerDisplay || diagnosis.Cumulative() {
		t.Fatalf("root=%s cumulative=%v summary=%s",
			diagnosis.Root, diagnosis.Cumulative(), diagnosis.Summary)
	}
}

func TestExactClockHasNoCumulativeError(t *testing.T) {
	rate := mustRate(t, "30000/1001")
	pipeline := Pipeline{
		Duration: 300 * time.Second,
		Preview:  ExactClock(rate),
		Export:   RenderClock{Rate: rate, Preroll: 2 * time.Second},
	}
	diagnosis, err := DiagnosePipeline(pipeline, standardCues())
	if err != nil {
		t.Fatalf("DiagnosePipeline: %v", err)
	}
	if diagnosis.Root != LayerNone || diagnosis.Cumulative() {
		t.Fatalf("root=%s summary=%s", diagnosis.Root, diagnosis.Summary)
	}
}

func TestReportedNominalRateIsFrameTimeDrift(t *testing.T) {
	actual := mustRate(t, "30000/1001")
	nominal := mustRate(t, "30")
	pipeline := Pipeline{
		Duration: 300 * time.Second,
		Preview:  ExactClock(actual),
		Export: RenderClock{
			Rate:         actual,
			ReportedRate: nominal,
		},
	}
	diagnosis, err := DiagnosePipeline(pipeline, standardCues())
	if err != nil {
		t.Fatalf("DiagnosePipeline: %v", err)
	}
	if diagnosis.Root != LayerFrameTime || !diagnosis.Cumulative() {
		t.Fatalf("root=%s cumulative=%v summary=%s",
			diagnosis.Root, diagnosis.Cumulative(), diagnosis.Summary)
	}
	if diagnosis.Slope <= 0 {
		t.Fatalf("rounding 30000/1001 down to 30 should make cues late, slope=%f", diagnosis.Slope)
	}
}

func TestIncludedPrerollIsAConstantNotAccumulation(t *testing.T) {
	rate := mustRate(t, "25")
	pipeline := Pipeline{
		Duration: 300 * time.Second,
		Preview:  ExactClock(rate),
		Export: RenderClock{
			Rate:           rate,
			Preroll:        2 * time.Second,
			IncludePreroll: true,
		},
	}
	diagnosis, err := DiagnosePipeline(pipeline, standardCues())
	if err != nil {
		t.Fatalf("DiagnosePipeline: %v", err)
	}
	if diagnosis.Root != LayerPreroll || diagnosis.Cumulative() {
		t.Fatalf("root=%s cumulative=%v summary=%s",
			diagnosis.Root, diagnosis.Cumulative(), diagnosis.Summary)
	}
}

func TestInfersRepeatedPrerollWithoutConfiguredSegment(t *testing.T) {
	rate := mustRate(t, "25")
	pipeline := Pipeline{
		Duration: 300 * time.Second,
		Preview:  ExactClock(rate),
		Export: RenderClock{
			Rate:           rate,
			Segment:        40 * time.Second,
			SegmentPreroll: 800 * time.Millisecond,
		},
	}
	comparisons, err := pipeline.CompareCues(standardCues())
	if err != nil {
		t.Fatalf("CompareCues: %v", err)
	}
	samples := make([]Sample, len(comparisons))
	for index, comparison := range comparisons {
		samples[index] = Sample{
			CueID:    comparison.CueID,
			Expected: comparison.Expected,
			Observed: comparison.Observed,
		}
	}
	cfg := ConfigForPipeline(pipeline)
	cfg.Segment = 0
	cfg.SegmentPreroll = 0
	diagnosis, err := DiagnoseSamples(samples, cfg)
	if err != nil {
		t.Fatalf("DiagnoseSamples: %v", err)
	}
	if diagnosis.Root != LayerPreroll || !diagnosis.Cumulative() {
		t.Fatalf("root=%s cumulative=%v summary=%s",
			diagnosis.Root, diagnosis.Cumulative(), diagnosis.Summary)
	}
	if absDuration(diagnosis.Segment-40*time.Second) > time.Second {
		t.Fatalf("inferred segment = %s, want about 40s", diagnosis.Segment)
	}
}

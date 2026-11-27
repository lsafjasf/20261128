// Package audit verifies a remapped render segment by segment.
//
// The audit never trusts a single global offset: for every curve segment
// it recomputes the expected source and output sample counts directly from
// the rate integral, checks the produced audio against those counts,
// round-trips timestamps through Map/Inverse, and proves that a constant
// shift applied to all mapped times cannot satisfy the segments at once.
package audit

// Severity levels for findings.
const (
	SevError   = "error"
	SevWarning = "warning"
)

// Finding is one audit problem.
type Finding struct {
	Severity string
	Where    string
	Message  string
}

// SegmentReport covers one curve segment in source and output terms.
type SegmentReport struct {
	Index              int
	SourceStart        float64
	SourceEnd          float64
	OutputStart        float64
	OutputEnd          float64
	RateStart          float64
	RateEnd            float64
	SourceSamples      int
	ExpectedOutSamples int
	ObservedOutSamples int
	MapInverseMaxErr   float64
	LocalRateCheck     bool
}

// OffsetCheck records the global-offset rejection experiment.
type OffsetCheck struct {
	Rejected   bool
	BestOffset float64
	Residual   float64
	Message    string
}

// PitchCheck compares measured pitch before and after resampling.
type PitchCheck struct {
	SourcePitchHz       float64
	LinearOutputPitchHz float64
	TSMOutputPitchHz    float64
	TSMDriftRatio       float64
	LinearDriftRatio    float64
	TSMPitchPreserved   bool
}

// Report is the full audit result.
type Report struct {
	Segments       []SegmentReport
	Findings       []Finding
	Offset         OffsetCheck
	Pitch          PitchCheck
	SubtitleOK     bool
	SubtitleDetail string
	Pass           bool
}

func (r *Report) add(sev, where, msg string) {
	r.Findings = append(r.Findings, Finding{Severity: sev, Where: where, Message: msg})
}

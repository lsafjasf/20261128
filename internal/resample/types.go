package resample

// Rate is a rational frame rate in frames per second.
type Rate struct {
	Num int `json:"num"`
	Den int `json:"den"`
}

// Strategy selects how output slots that do not land exactly on a source
// frame are populated.
//
// Mode:
//   - "repeat": nearest-prior source frame is duplicated (frame-rate
//     conversion by duplication only).
//   - "blend":  linear interpolation between the two bracketing source frames.
//
// MissingPolicy controls slots whose requested time falls inside a source
// gap: "hold" keeps the last available frame.
type Strategy struct {
	Mode          string `json:"mode"`
	Interpolation string `json:"interpolation"`
	MissingPolicy string `json:"missing_policy"`
}

// Gap describes one contiguous run of dropped source frames. All tick
// fields use the analysis timebase (TimebaseTicksPerSecond).
type Gap struct {
	AfterIndex             int `json:"after_index"`
	BeforeIndex            int `json:"before_index"`
	StartTicks             int `json:"start_ticks"`
	EndTicks               int `json:"end_ticks"`
	MissingFrames          int `json:"missing_frames"`
	NominalPeriodTicks     int `json:"nominal_period_ticks"`
	FirstRequestedPtsTicks int `json:"first_requested_pts_ticks"`
	LastRequestedPtsTicks  int `json:"last_requested_pts_ticks"`
}

// Frame is one output slot with the exact source frame(s) used to build it.
type Frame struct {
	OutputIndex       int       `json:"output_index"`
	PtsTicks          int       `json:"pts_ticks"`
	PtsMs             float64   `json:"pts_ms"`
	Kind              string    `json:"kind"`
	Duplicate         bool      `json:"duplicate"`
	Synthesized       bool      `json:"synthesized"`
	SourceIndices     []int     `json:"source_indices"`
	SourcePtsMs       []float64 `json:"source_pts_ms"`
	Alpha             float64   `json:"alpha"`
	TargetPtsMs       float64   `json:"target_pts_ms"`
	EffectivePtsTicks int       `json:"effective_pts_ticks"`
	EffectivePtsMs    float64   `json:"effective_pts_ms"`
	PhaseErrorMs      float64   `json:"phase_error_ms"`
	MissingSource     bool      `json:"missing_source"`
}

// CurvePoint is one sample of the judder curve: the observed motion step
// between two consecutive output frames versus the ideal constant step.
type CurvePoint struct {
	OutputIndex     int     `json:"output_index"`
	PtsMs           float64 `json:"pts_ms"`
	HasStep         bool    `json:"has_step"`
	ActualStepMs    float64 `json:"actual_step_ms"`
	IdealStepMs     float64 `json:"ideal_step_ms"`
	SignedErrorMs   float64 `json:"signed_error_ms"`
	NormalizedError float64 `json:"normalized_error"`
	Magnitude       float64 `json:"magnitude"`
	Class           string  `json:"class"`
	FrameKind       string  `json:"frame_kind"`
}

// Summary aggregates the frame accounting and judder statistics.
type Summary struct {
	OutputFrames              int     `json:"output_frames"`
	ExactFrames               int     `json:"exact_frames"`
	DuplicateFrames           int     `json:"duplicate_frames"`
	SynthesizedFrames         int     `json:"synthesized_frames"`
	GapHoldFrames             int     `json:"gap_hold_frames"`
	SourceEndHoldFrames       int     `json:"source_end_hold_frames"`
	MissingSourceFrames       int     `json:"missing_source_frames"`
	RepeatPauseFrames         int     `json:"repeat_pause_frames"`
	MissingSourcePauseFrames  int     `json:"missing_source_pause_frames"`
	InterpolationJudderFrames int     `json:"interpolation_judder_frames"`
	JudderFrames              int     `json:"judder_frames"`
	PeakNormalizedJudder      float64 `json:"peak_normalized_judder"`
	RMSNormalizedJudder       float64 `json:"rms_normalized_judder"`
}

// Report is the full per-strategy analysis result.
type Report struct {
	SourceRate          Rate         `json:"source_rate"`
	OutputRate          Rate         `json:"output_rate"`
	TimebaseTicksPerSec int          `json:"timebase_ticks_per_second"`
	SourcePeriodMs      float64      `json:"source_period_ms"`
	OutputPeriodMs      float64      `json:"output_period_ms"`
	Strategy            Strategy     `json:"strategy"`
	Gaps                []Gap        `json:"gaps"`
	Frames              []Frame      `json:"frames"`
	JudderCurve         []CurvePoint `json:"judder_curve"`
	Summary             Summary      `json:"summary"`
}

// Delta compares an "after" strategy against the "before" strategy.
type Delta struct {
	JudderFramesDelta         int     `json:"judder_frames_delta"`
	RepeatPauseDelta          int     `json:"repeat_pause_delta"`
	MissingSourcePauseDelta   int     `json:"missing_source_pause_delta"`
	InterpolationJudderDelta  int     `json:"interpolation_judder_delta"`
	RMSNormalizedJudderDelta  float64 `json:"rms_normalized_judder_delta"`
	PeakNormalizedJudderDelta float64 `json:"peak_normalized_judder_delta"`
}

// Comparison bundles both strategies and their delta.
type Comparison struct {
	Before Report `json:"before"`
	After  Report `json:"after"`
	Delta  Delta  `json:"delta"`
}

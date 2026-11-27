package analyze

const (
	VerdictClean        = "clean"
	VerdictMotionBlur   = "source_motion_blur"
	VerdictWeaveError   = "weave_error"
	VerdictOverSmooth   = "over_smoothed"
	VerdictInconclusive = "inconclusive"
)

type Shift struct {
	X          float64 `json:"x_px"`
	Y          float64 `json:"y_px"`
	Speed      float64 `json:"speed_px"`
	Direction  string  `json:"direction"`
	Confidence float64 `json:"confidence"`
}

type CombEvidence struct {
	EdgeFraction       float64 `json:"edge_fraction"`
	Magnitude          float64 `json:"magnitude"`
	SameParityResidual float64 `json:"same_parity_residual"`
	Score              float64 `json:"score"`
}

type BlurEvidence struct {
	EdgeWidthH       float64 `json:"edge_width_horizontal"`
	EdgeWidthV       float64 `json:"edge_width_vertical"`
	DetailH          float64 `json:"detail_horizontal"`
	DetailV          float64 `json:"detail_vertical"`
	Direction        string  `json:"direction"`
	DirectionalRatio float64 `json:"directional_ratio"`
}

type SmoothingEvidence struct {
	LineDuplicationRate float64 `json:"line_duplication_rate"`
	VerticalDetailRatio float64 `json:"vertical_detail_ratio"`
	TemporalDuplication float64 `json:"temporal_field_duplication"`
	Score               float64 `json:"score"`
}

type FieldEvidence struct {
	TopBottomMAD         float64 `json:"top_bottom_mad"`
	MotionCompensatedMAD float64 `json:"motion_compensated_mad"`
	ResidualRatio        float64 `json:"residual_ratio"`
	TemporalMAD          float64 `json:"temporal_mad"`
}

type MotionEvidence struct {
	IntraField Shift   `json:"intra_field"`
	InterFrame Shift   `json:"inter_frame"`
	Speed      float64 `json:"speed_px_per_field"`
	Direction  string  `json:"direction"`
	Confidence float64 `json:"confidence"`
}

type FieldOrderEvidence struct {
	Order      string  `json:"order"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
	Declared   string  `json:"declared,omitempty"`
}

type FrameEvidence struct {
	Combing   CombEvidence      `json:"combing"`
	Blur      BlurEvidence      `json:"blur"`
	Smoothing SmoothingEvidence `json:"smoothing"`
	Fields    FieldEvidence     `json:"fields"`
	Motion    MotionEvidence    `json:"motion"`
}

type FrameRecord struct {
	Type        string             `json:"type"`
	Frame       int                `json:"frame"`
	TimeSeconds float64            `json:"time_seconds"`
	Verdict     string             `json:"verdict"`
	Confidence  float64            `json:"confidence"`
	FieldOrder  FieldOrderEvidence `json:"field_order"`
	Scores      HypothesisScores   `json:"hypothesis_scores"`
	Evidence    FrameEvidence      `json:"evidence"`
	Reasons     []string           `json:"reasons"`
}

type HypothesisScores struct {
	Weave      float64 `json:"weave_error"`
	MotionBlur float64 `json:"source_motion_blur"`
	OverSmooth float64 `json:"over_smoothed"`
}

type Summary struct {
	Type       string         `json:"type"`
	Frames     int            `json:"frames"`
	Counts     map[string]int `json:"counts"`
	Dominant   string         `json:"dominant_verdict"`
	FieldOrder string         `json:"field_order"`
	OrderConf  float64        `json:"field_order_confidence"`
}

type Document struct {
	Type      string        `json:"type"`
	Input     string        `json:"input,omitempty"`
	Width     int           `json:"width"`
	Height    int           `json:"height"`
	FrameRate float64       `json:"frame_rate"`
	Frames    []FrameRecord `json:"frames"`
	Summary   Summary       `json:"summary"`
}

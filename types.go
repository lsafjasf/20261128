package ghostdiag

// Options controls temporal and spatial evidence thresholds. Zero values are
// replaced by DefaultOptions when Analyze is called.
type Options struct {
	GhostThreshold     float64 `json:"ghost_threshold"`
	NoiseThreshold     int     `json:"noise_threshold"`
	MotionThreshold    int     `json:"motion_threshold"`
	HistoryFrames      int     `json:"history_frames"`
	MinArea            int     `json:"min_area"`
	BlockSize          int     `json:"block_size"`
	BlockThreshold     float64 `json:"block_threshold"`
	BlurWidthThreshold float64 `json:"blur_width_threshold"`
	MaxFindings        int     `json:"max_findings"`
}

// DefaultOptions returns conservative thresholds for 8-bit luma footage.
func DefaultOptions() Options {
	return Options{
		GhostThreshold:     0.52,
		NoiseThreshold:     7,
		MotionThreshold:    12,
		HistoryFrames:      5,
		MinArea:            24,
		BlockSize:          8,
		BlockThreshold:     0.38,
		BlurWidthThreshold: 2.35,
		MaxFindings:        8,
	}
}

func (o Options) normalized() Options {
	def := DefaultOptions()
	if o.GhostThreshold <= 0 {
		o.GhostThreshold = def.GhostThreshold
	}
	if o.NoiseThreshold <= 0 {
		o.NoiseThreshold = def.NoiseThreshold
	}
	if o.MotionThreshold <= 0 {
		o.MotionThreshold = def.MotionThreshold
	}
	if o.HistoryFrames <= 0 {
		o.HistoryFrames = def.HistoryFrames
	}
	if o.MinArea <= 0 {
		o.MinArea = def.MinArea
	}
	if o.BlockSize <= 0 {
		o.BlockSize = def.BlockSize
	}
	if o.BlockThreshold <= 0 {
		o.BlockThreshold = def.BlockThreshold
	}
	if o.BlurWidthThreshold <= 0 {
		o.BlurWidthThreshold = def.BlurWidthThreshold
	}
	if o.MaxFindings <= 0 {
		o.MaxFindings = def.MaxFindings
	}
	return o
}

// Region is an inclusive-exclusive bounding box and its supporting pixel count.
type Region struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"w"`
	Height int `json:"h"`
	Area   int `json:"area"`
}

// Evidence values are normalized to 0..1 unless the field name says otherwise.
type Evidence struct {
	TemporalScore  float64 `json:"temporal_score"`
	BlockScore     float64 `json:"block_score"`
	BlurWidth      float64 `json:"blur_width_px"`
	MotionOverlap  float64 `json:"motion_overlap"`
	SourceChange   float64 `json:"source_change_mad"`
	ResidualChange float64 `json:"residual_mad"`
}

// Finding is one classified connected artifact region.
type Finding struct {
	Frame          int      `json:"frame"`
	Timestamp      float64  `json:"time_seconds"`
	Classification string   `json:"classification"`
	Score          float64  `json:"score"`
	Region         Region   `json:"region"`
	Evidence       Evidence `json:"evidence"`
	Secondary      []string `json:"secondary,omitempty"`
	Reasons        []string `json:"reasons"`
}

// FrameResult is the per-frame comparison of original and denoised luma.
type FrameResult struct {
	Frame              int       `json:"frame"`
	Timestamp          float64   `json:"time_seconds"`
	Trajectory         Box       `json:"trajectory"`
	MotionPixels       int       `json:"motion_pixels"`
	SourceBlurWidth    float64   `json:"source_blur_width_px"`
	CodingBlockScore   float64   `json:"coding_block_score"`
	BackgroundResidual float64   `json:"static_background_residual_mad"`
	SourceNoise        float64   `json:"estimated_source_noise_mad"`
	NoiseReduction     float64   `json:"static_noise_reduction"`
	Findings           []Finding `json:"findings,omitempty"`
}

// InputInfo binds a run to the exact pictures used for every parameter set.
type InputInfo struct {
	SHA256    string  `json:"sha256"`
	Frames    int     `json:"frames"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	FrameRate float64 `json:"frame_rate"`
}

// Summary condenses frame counts and artifact classifications.
type Summary struct {
	Frames          int            `json:"frames"`
	Counts          map[string]int `json:"counts"`
	FirstGhostFrame *int           `json:"first_ghost_frame"`
}

// Report is the deterministic result of one comparison run.
type Report struct {
	Schema     string        `json:"schema"`
	Input      InputInfo     `json:"input"`
	Options    Options       `json:"options"`
	Frames     []FrameResult `json:"frames"`
	FirstGhost *Finding      `json:"first_ghost,omitempty"`
	Summary    Summary       `json:"summary"`
}

const reportSchema = "ghostdiag/report/v1"

package analysis

// Options controls the reference conversion and time-domain analysis.
type Options struct {
	CutoffHz           float64
	ReferenceHalfWidth int
	FFTSize            int
	MaxTransients      int
	BoundaryWindowMs   float64
	MinCoherence       float64
}

func DefaultOptions(srcRate, dstRate int) Options {
	nyq := srcRate
	if dstRate < nyq {
		nyq = dstRate
	}
	return Options{
		CutoffHz:           0.95 * float64(nyq) / 2,
		ReferenceHalfWidth: 64,
		FFTSize:            32768,
		MaxTransients:      12,
		BoundaryWindowMs:   50,
		MinCoherence:       0.85,
	}
}

type StreamInfo struct {
	SampleRate int     `json:"sample_rate"`
	Samples    int     `json:"samples"`
	DurationMS float64 `json:"duration_ms"`
	Peak       float64 `json:"peak"`
	RMS        float64 `json:"rms"`
}

type AlignmentReport struct {
	LagSamples float64 `json:"lag_samples"`
	LagMS      float64 `json:"lag_ms"`
	Confidence float64 `json:"confidence"`
	Method     string  `json:"method"`
	Warning    string  `json:"warning,omitempty"`
}

type BandDelay struct {
	Name          string  `json:"name"`
	LowHz         float64 `json:"low_hz"`
	HighHz        float64 `json:"high_hz"`
	GroupDelayMS  float64 `json:"relative_group_delay_ms"`
	TotalDelayMS  float64 `json:"latency_referenced_group_delay_ms"`
	RippleMS      float64 `json:"ripple_p95_p5_ms"`
	PhaseRMSError float64 `json:"phase_rms_error_deg"`
	Coherence     float64 `json:"median_coherence"`
	Bins          int     `json:"valid_bins"`
}

type GroupDelayReport struct {
	Bands                 []BandDelay `json:"bands"`
	MedianDelayMS         float64     `json:"median_relative_delay_ms"`
	RippleMS              float64     `json:"passband_ripple_p95_p5_ms"`
	PhaseLinearityRMSDeg  float64     `json:"phase_linearity_rms_deg"`
	ReferenceFitDelayMS   float64     `json:"reference_fit_delay_ms"`
	CoherenceGate         float64     `json:"coherence_gate"`
	ValidPassbandBins     int         `json:"valid_passband_bins"`
	EffectiveBandwidthHz  float64     `json:"effective_bandwidth_3db_hz"`
	PhaseCurveIsReliable  bool        `json:"phase_curve_reliable"`
	AggregateSpectrumUsed bool        `json:"aggregate_spectrum_similarity_used"`
}

type ImpulseReport struct {
	Support40DBMS      float64 `json:"support_40db_ms"`
	Support60DBMS      float64 `json:"support_60db_ms"`
	Energy99MS         float64 `json:"energy_99pct_ms"`
	Support60Samples   float64 `json:"support_60db_destination_samples"`
	PreEnergyDB        float64 `json:"pre_energy_db_rel_post"`
	Symmetry           float64 `json:"symmetry"`
	PeakResidualMS     float64 `json:"peak_residual_ms"`
	LinearPhaseLikely  bool    `json:"linear_phase_likely"`
	LatencyImpliedTaps float64 `json:"latency_implied_taps_if_uncompensated"`
	TapsNote           string  `json:"taps_note"`
}

type TransientEvent struct {
	TimeSec         float64 `json:"time_sec"`
	SourceWidthMS   float64 `json:"source_width_ms"`
	RefWidthMS      float64 `json:"reference_width_ms"`
	DstWidthMS      float64 `json:"destination_width_ms"`
	WidthRatio      float64 `json:"destination_over_reference"`
	RefRiseMS       float64 `json:"reference_rise_ms"`
	DstRiseMS       float64 `json:"destination_rise_ms"`
	RiseRatio       float64 `json:"destination_over_reference_rise"`
	LeadShiftMS     float64 `json:"lead_shift_ms"`
	TailShiftMS     float64 `json:"tail_shift_ms"`
	LeadPhaseDeg    float64 `json:"lead_phase_error_deg"`
	TailPhaseDeg    float64 `json:"tail_phase_error_deg"`
	RefPrePostDB    float64 `json:"reference_pre_post_db"`
	DstPrePostDB    float64 `json:"destination_pre_post_db"`
	SourcePrePostDB float64 `json:"source_pre_post_db"`
}

type TransientSummary struct {
	Events             []TransientEvent `json:"events"`
	MedianWidthRatio   float64          `json:"median_width_ratio"`
	P90WidthRatio      float64          `json:"p90_width_ratio"`
	MedianRiseRatio    float64          `json:"median_rise_ratio"`
	MedianLeadShiftMS  float64          `json:"median_lead_shift_ms"`
	MedianTailShiftMS  float64          `json:"median_tail_shift_ms"`
	SourceAlreadyBroad bool             `json:"source_already_broad"`
	ConverterWidening  bool             `json:"converter_widening"`
	DetectedCount      int              `json:"detected_count"`
}

type BoundaryScore struct {
	Mode      string  `json:"mode"`
	HalfWidth int     `json:"reference_half_width_source_samples"`
	Start     float64 `json:"start_nrmse"`
	End       float64 `json:"end_nrmse"`
	Combined  float64 `json:"combined_nrmse"`
}

type BoundaryReport struct {
	WindowMS        float64         `json:"window_ms"`
	Scores          []BoundaryScore `json:"scores"`
	BestMode        string          `json:"best_mode"`
	BestHalfWidth   int             `json:"best_half_width_source_samples"`
	BestMargin      float64         `json:"best_margin"`
	Indeterminate   bool            `json:"indeterminate"`
	LeadingExtraMS  float64         `json:"leading_extra_ms"`
	TrailingExtraMS float64         `json:"trailing_extra_ms"`
	InteriorNRMSE   float64         `json:"interior_nrmse"`
}

type Attribution struct {
	Classification string   `json:"classification"`
	Confidence     float64  `json:"confidence"`
	Evidence       []string `json:"evidence"`
	Limitations    []string `json:"limitations"`
}

type Report struct {
	Source      StreamInfo       `json:"source"`
	Destination StreamInfo       `json:"destination"`
	Conversion  string           `json:"conversion"`
	Alignment   AlignmentReport  `json:"alignment"`
	GroupDelay  GroupDelayReport `json:"group_delay"`
	Impulse     ImpulseReport    `json:"impulse_response"`
	Transients  TransientSummary `json:"transients"`
	Boundary    BoundaryReport   `json:"boundary"`
	Attribution Attribution      `json:"attribution"`
	Warnings    []string         `json:"warnings,omitempty"`
}

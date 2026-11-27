package subburn

import (
	"fmt"
	"math"
	"sort"
	"time"
)

type Layer string

const (
	LayerNone      Layer = "none"
	LayerFrameTime Layer = "frame_timebase"
	LayerOrigin    Layer = "subtitle_origin"
	LayerPreroll   Layer = "encoder_preroll"
	LayerDisplay   Layer = "output_display_time"
	LayerUnknown   Layer = "unknown"
)

// Sample is one cue-onset measurement. A negative Error means the final video
// showed the cue earlier than the preview.
type Sample struct {
	CueID    string
	Expected time.Duration
	Observed time.Duration
}

func (s Sample) Error() time.Duration {
	return s.Observed - s.Expected
}

type AuditConfig struct {
	PreviewRate  Rate
	ExportRate   Rate
	FrameStep    time.Duration
	ReportedRate Rate
	Preroll      time.Duration
	Segment      time.Duration

	SegmentPreroll    time.Duration
	PrerollInSubtitle bool
	SubtitleOrigin    time.Duration
	DisplayStart      time.Duration
	DisplayInSubtitle bool
}

type Finding struct {
	Layer   Layer  `json:"layer"`
	Message string `json:"message"`
}

type Diagnosis struct {
	Root         Layer         `json:"root"`
	Summary      string        `json:"summary"`
	Accumulates  bool          `json:"accumulates"`
	Slope        float64       `json:"slope"`
	Constant     time.Duration `json:"constant_ns"`
	MaxAbsError  time.Duration `json:"max_abs_error_ns"`
	DriftPerMin  time.Duration `json:"drift_per_minute_ns"`
	LinearRMSE   time.Duration `json:"linear_rmse_ns"`
	StepRMSE     time.Duration `json:"step_rmse_ns"`
	StepFitted   bool          `json:"step_fitted"`
	Segment      time.Duration `json:"segment_ns"`
	Quantization time.Duration `json:"quantization_ns"`
	Findings     []Finding     `json:"findings"`
	SampleCount  int           `json:"sample_count"`
}

func (d Diagnosis) Cumulative() bool {
	return d.Accumulates
}

func ConfigForPipeline(p Pipeline) AuditConfig {
	return AuditConfig{
		PreviewRate:       p.Preview.Rate,
		ExportRate:        p.Export.Rate,
		FrameStep:         p.Export.FrameStep,
		ReportedRate:      p.Export.ReportedRate,
		Preroll:           p.Export.Preroll,
		Segment:           p.Export.Segment,
		SegmentPreroll:    p.Export.SegmentPreroll,
		PrerollInSubtitle: p.Export.IncludePreroll,
		SubtitleOrigin:    p.Export.SubtitleOrigin,
		DisplayStart:      p.Export.DisplayStart,
		DisplayInSubtitle: p.Export.IncludeDisplayStart,
	}
}

func DiagnosePipeline(p Pipeline, cues []Cue) (Diagnosis, error) {
	comparisons, err := p.CompareCues(cues)
	if err != nil {
		return Diagnosis{}, err
	}
	samples := make([]Sample, len(comparisons))
	for i, comparison := range comparisons {
		samples[i] = Sample{
			CueID:    comparison.CueID,
			Expected: comparison.Expected,
			Observed: comparison.Observed,
		}
	}
	return DiagnoseSamples(samples, ConfigForPipeline(p))
}

// DiagnoseSamples fits whole-timeline models to all cue onsets. It deliberately
// does not compensate the samples: the goal is to name the layer that produced
// them.
func DiagnoseSamples(samples []Sample, cfg AuditConfig) (Diagnosis, error) {
	if len(samples) < 2 {
		return Diagnosis{}, fmt.Errorf("at least two cue-onset samples are required")
	}
	if err := validateAuditConfig(cfg); err != nil {
		return Diagnosis{}, err
	}
	ordered := append([]Sample(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Expected < ordered[j].Expected
	})
	for i := 1; i < len(ordered); i++ {
		if ordered[i].Expected == ordered[i-1].Expected {
			return Diagnosis{}, fmt.Errorf("duplicate expected time %s", ordered[i].Expected)
		}
	}

	times := make([]float64, len(ordered))
	errors := make([]float64, len(ordered))
	var maxAbs time.Duration
	for i, sample := range ordered {
		if sample.Expected < 0 || sample.Observed < 0 {
			return Diagnosis{}, fmt.Errorf("sample %q has negative time", sample.CueID)
		}
		times[i] = sample.Expected.Seconds()
		errors[i] = sample.Error().Seconds()
		if abs := absDuration(sample.Error()); abs > maxAbs {
			maxAbs = abs
		}
	}

	slope, intercept, linearRMSE := linearFit(times, errors)
	_, constantRMSE := constantFit(errors)
	quantization := maxDuration(cfg.PreviewRate.FrameDuration(), cfg.ExportRate.FrameDuration())
	if quantization <= 0 {
		quantization = time.Millisecond
	}
	tolerance := 3 * quantization / 2
	segmentSeconds := cfg.Segment.Seconds()
	inferredSegment := false
	if segmentSeconds <= 0 {
		if inferred, ok := inferSegmentPeriod(times, errors, tolerance.Seconds()); ok {
			segmentSeconds = inferred
			inferredSegment = true
		}
	}
	stepSlope, stepRMSE, stepLevels := segmentFit(times, errors, segmentSeconds)
	stepFitted := segmentSeconds > 0 && stepLevels >= 2

	diagnosis := Diagnosis{
		Root:         LayerUnknown,
		Slope:        slope,
		Constant:     secondsToDuration(intercept),
		MaxAbsError:  maxAbs,
		DriftPerMin:  secondsToDuration(slope * 60),
		LinearRMSE:   secondsToDuration(linearRMSE),
		StepRMSE:     secondsToDuration(stepRMSE),
		StepFitted:   stepFitted,
		Segment:      secondsToDuration(segmentSeconds),
		Quantization: quantization,
		SampleCount:  len(ordered),
	}
	if !stepFitted {
		diagnosis.StepRMSE = 0
	}
	diagnosis.Findings = configFindings(cfg)

	span := times[len(times)-1] - times[0]
	cumulativeLinear := math.Abs(slope)*span > 2*tolerance.Seconds()
	linearClean := secondsToDuration(linearRMSE) <= tolerance
	stepDiff := math.Abs(stepSlope) * float64(stepLevels-1)
	stepClean := stepLevels >= 2 && stepDiff > 2*tolerance.Seconds() &&
		secondsToDuration(stepRMSE) <= tolerance &&
		stepRMSE < linearRMSE*0.8

	if stepClean {
		diagnosis.Root = LayerPreroll
		diagnosis.Accumulates = true
		diagnosis.Summary = "cue error changes in flat steps at encoder segment boundaries"
		if inferredSegment {
			diagnosis.Findings = append(diagnosis.Findings, Finding{
				Layer:   LayerPreroll,
				Message: fmt.Sprintf("inferred encoder segment period is about %s", diagnosis.Segment),
			})
		}
		diagnosis.Findings = append(diagnosis.Findings, Finding{
			Layer: LayerPreroll,
			Message: fmt.Sprintf(
				"segments add about %s of lead-in; %d distinct segment levels were observed",
				secondsToDuration(math.Abs(stepSlope)), stepLevels),
		})
		return diagnosis, nil
	}

	if cumulativeLinear && linearClean {
		diagnosis.Root = LayerFrameTime
		diagnosis.Accumulates = true
		diagnosis.Summary = "cue error is linear in source time, so the frame/timestamp clock accumulates drift"
		diagnosis.Findings = append(diagnosis.Findings, Finding{
			Layer: LayerFrameTime,
			Message: fmt.Sprintf(
				"fitted drift is %s per minute (slope %.6f%%, linear RMSE %s)",
				diagnosis.DriftPerMin, slope*100, diagnosis.LinearRMSE),
		})
		return diagnosis, nil
	}

	if maxAbs <= tolerance {
		diagnosis.Root = LayerNone
		diagnosis.Summary = "cue onsets stay within one display frame; no cumulative timing error"
		return diagnosis, nil
	}

	if math.Abs(slope)*span <= 2*tolerance.Seconds() && constantRMSE <= tolerance.Seconds() {
		diagnosis.Root = constantLayer(cfg, diagnosis.Constant, tolerance)
		diagnosis.Summary = "cue error is constant; this is an origin/display offset, not accumulated drift"
		diagnosis.Findings = append(diagnosis.Findings, Finding{
			Layer:   diagnosis.Root,
			Message: fmt.Sprintf("all samples are shifted by about %s", diagnosis.Constant),
		})
		return diagnosis, nil
	}

	if cfg.FrameStep > 0 || cfg.ReportedRate.Valid() ||
		cfg.ExportRate != cfg.PreviewRate {
		diagnosis.Root = LayerFrameTime
		diagnosis.Accumulates = true
		diagnosis.Summary = "frame timestamp configuration is inconsistent, but the samples are not cleanly linear"
		return diagnosis, nil
	}
	diagnosis.Summary = "samples do not match a single constant, linear, or segment-step model"
	return diagnosis, nil
}

func validateAuditConfig(cfg AuditConfig) error {
	if !cfg.PreviewRate.Valid() {
		return fmt.Errorf("invalid preview rate: %s", cfg.PreviewRate)
	}
	if !cfg.ExportRate.Valid() {
		return fmt.Errorf("invalid export rate: %s", cfg.ExportRate)
	}
	if cfg.FrameStep < 0 || cfg.Preroll < 0 || cfg.Segment < 0 ||
		cfg.SegmentPreroll < 0 || cfg.DisplayStart < 0 {
		return fmt.Errorf("timing offsets must not be negative")
	}
	if (cfg.ReportedRate.Num != 0 || cfg.ReportedRate.Den != 0) &&
		!cfg.ReportedRate.Valid() {
		return fmt.Errorf("invalid reported rate: %s", cfg.ReportedRate)
	}
	if cfg.SegmentPreroll > 0 && cfg.Segment <= 0 {
		return fmt.Errorf("segment preroll requires a segment duration")
	}
	return nil
}

func configFindings(cfg AuditConfig) []Finding {
	var findings []Finding
	if cfg.PreviewRate != cfg.ExportRate {
		findings = append(findings, Finding{
			Layer: LayerFrameTime,
			Message: fmt.Sprintf("preview rate %s differs from export rate %s",
				cfg.PreviewRate, cfg.ExportRate),
		})
	}
	if cfg.FrameStep > 0 {
		exact := cfg.ExportRate.FrameDuration()
		if absDuration(cfg.FrameStep-exact) > 100*time.Microsecond {
			findings = append(findings, Finding{
				Layer: LayerFrameTime,
				Message: fmt.Sprintf("frame timestamps use a fixed %s step instead of exact %s frame duration",
					cfg.FrameStep, exact),
			})
		}
	}
	if cfg.ReportedRate.Valid() && !cfg.ReportedRate.Equal(cfg.ExportRate) {
		findings = append(findings, Finding{
			Layer: LayerFrameTime,
			Message: fmt.Sprintf("frame timestamps use reported rate %s instead of real rate %s",
				cfg.ReportedRate, cfg.ExportRate),
		})
	}
	if cfg.Preroll > 0 && cfg.PrerollInSubtitle {
		findings = append(findings, Finding{
			Layer:   LayerPreroll,
			Message: fmt.Sprintf("encoder preroll %s is included in subtitle lookup time", cfg.Preroll),
		})
	}
	if cfg.SegmentPreroll > 0 {
		findings = append(findings, Finding{
			Layer: LayerPreroll,
			Message: fmt.Sprintf("preroll %s is added for every %s segment",
				cfg.SegmentPreroll, cfg.Segment),
		})
	}
	if cfg.SubtitleOrigin != 0 {
		findings = append(findings, Finding{
			Layer:   LayerOrigin,
			Message: fmt.Sprintf("subtitle origin %s is not normalized", cfg.SubtitleOrigin),
		})
	}
	if cfg.DisplayStart > 0 && cfg.DisplayInSubtitle {
		findings = append(findings, Finding{
			Layer:   LayerDisplay,
			Message: fmt.Sprintf("container display start %s is included in subtitle lookup time", cfg.DisplayStart),
		})
	}
	return findings
}

func constantLayer(cfg AuditConfig, constant, tolerance time.Duration) Layer {
	candidates := []struct {
		layer  Layer
		offset time.Duration
		active bool
	}{
		{LayerPreroll, cfg.Preroll, cfg.PrerollInSubtitle},
		{LayerDisplay, cfg.DisplayStart, cfg.DisplayInSubtitle},
		{LayerOrigin, cfg.SubtitleOrigin, cfg.SubtitleOrigin != 0},
	}
	for _, candidate := range candidates {
		if !candidate.active {
			continue
		}
		if absDuration(absDuration(constant)-absDuration(candidate.offset)) <= tolerance {
			return candidate.layer
		}
	}
	if cfg.SubtitleOrigin != 0 {
		return LayerOrigin
	}
	if cfg.DisplayStart > 0 {
		return LayerDisplay
	}
	return LayerOrigin
}

func linearFit(x, y []float64) (slope, intercept, rmse float64) {
	var meanX, meanY float64
	for i := range x {
		meanX += x[i]
		meanY += y[i]
	}
	meanX /= float64(len(x))
	meanY /= float64(len(y))
	var sxx, sxy float64
	for i := range x {
		dx := x[i] - meanX
		sxx += dx * dx
		sxy += dx * (y[i] - meanY)
	}
	if sxx == 0 {
		return 0, meanY, rmsFit(x, y, func(float64) float64 { return meanY })
	}
	slope = sxy / sxx
	intercept = meanY - slope*meanX
	return slope, intercept, rmsFit(x, y, func(i float64) float64 {
		return slope*i + intercept
	})
}

func constantFit(y []float64) (constant, rmse float64) {
	for _, value := range y {
		constant += value
	}
	constant /= float64(len(y))
	return constant, rmsFit(nil, y, func(float64) float64 { return constant })
}

func segmentFit(times, errors []float64, segmentSeconds float64) (slope float64, rmse float64, levels int) {
	if segmentSeconds <= 0 {
		return 0, math.Inf(1), 0
	}
	indices := make([]float64, len(times))
	minLevel, maxLevel := int64(math.MaxInt64), int64(math.MinInt64)
	for i, at := range times {
		level := int64(math.Floor(at / segmentSeconds))
		indices[i] = float64(level)
		if level < minLevel {
			minLevel = level
		}
		if level > maxLevel {
			maxLevel = level
		}
	}
	if minLevel == math.MaxInt64 {
		return 0, math.Inf(1), 0
	}
	slope, _, rmse = linearFit(indices, errors)
	levels = int(maxLevel - minLevel + 1)
	if levels < 2 {
		return slope, math.Inf(1), levels
	}
	return slope, rmse, levels
}

// inferSegmentPeriod detects a staircase without relying on an audit config.
// Linear drift changes every sample; a repeated-preroll bug has long flat runs
// interrupted by one jump per segment.
func inferSegmentPeriod(times, errors []float64, tolerance float64) (float64, bool) {
	if len(times) < 4 || tolerance <= 0 {
		return 0, false
	}
	flatThreshold := 0.35 * tolerance
	jumpThreshold := 1.25 * tolerance
	flat, jumps := 0, 0
	var jumpPositions []float64
	for i := 1; i < len(times); i++ {
		delta := errors[i] - errors[i-1]
		switch {
		case math.Abs(delta) <= flatThreshold:
			flat++
		case math.Abs(delta) > jumpThreshold:
			jumps++
			jumpPositions = append(jumpPositions, (times[i]+times[i-1])/2)
		}
	}
	changes := len(times) - 1
	if jumps < 2 || flat < changes/4 || jumps > changes*3/4 {
		return 0, false
	}
	spacings := make([]float64, 0, len(jumpPositions)-1)
	for i := 1; i < len(jumpPositions); i++ {
		spacing := jumpPositions[i] - jumpPositions[i-1]
		if spacing > tolerance {
			spacings = append(spacings, spacing)
		}
	}
	if len(spacings) == 0 {
		return 0, false
	}
	sort.Float64s(spacings)
	return spacings[len(spacings)/2], true
}

func rmsFit(x, y []float64, predict func(float64) float64) float64 {
	var sum float64
	for i, value := range y {
		at := float64(0)
		if len(x) == len(y) {
			at = x[i]
		}
		delta := value - predict(at)
		sum += delta * delta
	}
	return math.Sqrt(sum / float64(len(y)))
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func secondsToDuration(seconds float64) time.Duration {
	const limit = float64(math.MaxInt64)
	ns := seconds * float64(time.Second)
	if ns > limit {
		return time.Duration(math.MaxInt64)
	}
	if ns < -limit {
		return time.Duration(math.MinInt64)
	}
	return time.Duration(ns)
}

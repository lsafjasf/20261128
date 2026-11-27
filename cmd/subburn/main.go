package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"subburn"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "subburn:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "demo":
		return runDemo()
	case "audit":
		return runAudit(args[1:])
	case "plan":
		return runPlan(args[1:])
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, usage)
		return nil
	default:
		return usageError()
	}
}

const usage = `usage:
  subburn demo
  subburn audit [-json] <samples.json>
  subburn plan -rate 30000/1001 -duration 5m -srt in.srt [-preroll 2s] [-json]
`

func usageError() error {
	fmt.Fprint(os.Stderr, usage)
	return errors.New("unknown or missing command")
}

func runDemo() error {
	rate, err := subburn.ParseRate("30000/1001")
	if err != nil {
		return err
	}
	duration := 5 * time.Minute
	cues := make([]subburn.Cue, 0, 13)
	for index := 0; index < 13; index++ {
		start := time.Duration(10+20*index) * time.Second
		cues = append(cues, subburn.Cue{
			ID:    fmt.Sprintf("cue-%02d", index+1),
			Start: start,
			End:   start + 2*time.Second,
			Text:  "subtitle",
		})
	}
	broken := subburn.Pipeline{
		Duration: duration,
		Preview:  subburn.ExactClock(rate),
		Export: subburn.RenderClock{
			Rate:      rate,
			FrameStep: 34 * time.Millisecond,
			Preroll:   2 * time.Second,
		},
	}
	comparisons, err := broken.CompareCues(cues)
	if err != nil {
		return err
	}
	diagnosis, err := subburn.DiagnosePipeline(broken, cues)
	if err != nil {
		return err
	}

	fmt.Printf("rate: preview=%s export=%s exact-frame=%s\n",
		broken.Preview.Rate, broken.Export.Rate, rate.FrameDuration())
	fmt.Println("cue       expected      final        error")
	for index, comparison := range comparisons {
		if index != 0 && index != len(comparisons)/2 && index != len(comparisons)-1 {
			continue
		}
		fmt.Printf("%-8s  %-12s  %-12s  %s\n",
			comparison.CueID, formatTime(comparison.Expected),
			formatTime(comparison.Observed), formatSigned(comparison.Error))
	}
	fmt.Printf("root: %s cumulative=%v\n", diagnosis.Root, diagnosis.Cumulative())
	fmt.Printf("slope: %.4f%%, drift=%s per minute, linear-rmse=%s\n",
		diagnosis.Slope*100, formatSigned(diagnosis.DriftPerMin), diagnosis.LinearRMSE)
	fmt.Println("cause:", diagnosis.Summary)

	corrected := broken.Corrected()
	plan, err := subburn.BuildBurnPlan(corrected.Export.Rate,
		corrected.Export.Preroll, corrected.Duration, cues)
	if err != nil {
		return err
	}
	if err := plan.Verify(corrected.Preview, cues); err != nil {
		return err
	}
	fixed, err := corrected.CompareCues(cues)
	if err != nil {
		return err
	}
	var maxError time.Duration
	for _, comparison := range fixed {
		if absolute(comparison.Error) > maxError {
			maxError = absolute(comparison.Error)
		}
	}
	fmt.Printf("fix: exact PTS + preroll excluded from lookup; cue starts unchanged; max residual=%s\n", maxError)
	if diagnosis.Root != subburn.LayerFrameTime || !diagnosis.Cumulative() {
		return fmt.Errorf("demo diagnosis = %s, want frame_timebase", diagnosis.Root)
	}
	return nil
}

type auditFile struct {
	PreviewRate       string       `json:"preview_rate"`
	ExportRate        string       `json:"export_rate"`
	FrameStepMS       int64        `json:"frame_step_ms"`
	ReportedRate      string       `json:"reported_rate"`
	PrerollMS         int64        `json:"preroll_ms"`
	PrerollInSubtitle bool         `json:"preroll_in_subtitle"`
	SegmentMS         int64        `json:"segment_ms"`
	SegmentPrerollMS  int64        `json:"segment_preroll_ms"`
	SubtitleOriginMS  int64        `json:"subtitle_origin_ms"`
	DisplayStartMS    int64        `json:"display_start_ms"`
	DisplayInSubtitle bool         `json:"display_in_subtitle"`
	Samples           []sampleFile `json:"samples"`
}

type sampleFile struct {
	ID         string `json:"id"`
	ExpectedMS int64  `json:"expected_ms"`
	ObservedMS int64  `json:"observed_ms"`
}

func runAudit(args []string) error {
	flags := flag.NewFlagSet("audit", flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("audit requires one JSON file")
	}
	data, err := readInput(flags.Arg(0))
	if err != nil {
		return err
	}
	var input auditFile
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("parse audit JSON: %w", err)
	}
	previewRate, err := subburn.ParseRate(input.PreviewRate)
	if err != nil {
		return fmt.Errorf("preview_rate: %w", err)
	}
	exportRate, err := subburn.ParseRate(input.ExportRate)
	if err != nil {
		return fmt.Errorf("export_rate: %w", err)
	}
	cfg := subburn.AuditConfig{
		PreviewRate:       previewRate,
		ExportRate:        exportRate,
		FrameStep:         time.Duration(input.FrameStepMS) * time.Millisecond,
		Preroll:           time.Duration(input.PrerollMS) * time.Millisecond,
		Segment:           time.Duration(input.SegmentMS) * time.Millisecond,
		SegmentPreroll:    time.Duration(input.SegmentPrerollMS) * time.Millisecond,
		PrerollInSubtitle: input.PrerollInSubtitle,
		SubtitleOrigin:    time.Duration(input.SubtitleOriginMS) * time.Millisecond,
		DisplayStart:      time.Duration(input.DisplayStartMS) * time.Millisecond,
		DisplayInSubtitle: input.DisplayInSubtitle,
	}
	if input.ReportedRate != "" {
		cfg.ReportedRate, err = subburn.ParseRate(input.ReportedRate)
		if err != nil {
			return fmt.Errorf("reported_rate: %w", err)
		}
	}
	if len(input.Samples) < 2 {
		return errors.New("audit requires at least two samples")
	}
	samples := make([]subburn.Sample, len(input.Samples))
	for index, sample := range input.Samples {
		samples[index] = subburn.Sample{
			CueID:    sample.ID,
			Expected: time.Duration(sample.ExpectedMS) * time.Millisecond,
			Observed: time.Duration(sample.ObservedMS) * time.Millisecond,
		}
	}
	diagnosis, err := subburn.DiagnoseSamples(samples, cfg)
	if err != nil {
		return err
	}
	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(diagnosis)
	}
	fmt.Printf("root: %s cumulative=%v\n", diagnosis.Root, diagnosis.Cumulative())
	fmt.Println("summary:", diagnosis.Summary)
	fmt.Printf("samples=%d slope=%.6f%% drift=%s/min max-abs-error=%s\n",
		diagnosis.SampleCount, diagnosis.Slope*100,
		formatSigned(diagnosis.DriftPerMin), diagnosis.MaxAbsError)
	stepRMSE := "n/a"
	if diagnosis.StepFitted {
		stepRMSE = diagnosis.StepRMSE.String()
	}
	fmt.Printf("linear-rmse=%s step-rmse=%s quantization=%s\n",
		diagnosis.LinearRMSE, stepRMSE, diagnosis.Quantization)
	for _, finding := range diagnosis.Findings {
		fmt.Printf("- [%s] %s\n", finding.Layer, finding.Message)
	}
	return nil
}

type planJSON struct {
	Rate          string          `json:"rate"`
	PrerollNS     int64           `json:"preroll_ns"`
	DurationNS    int64           `json:"duration_ns"`
	SubtitleQuery string          `json:"subtitle_query"`
	Cues          []placementJSON `json:"cues"`
}

type placementJSON struct {
	ID             string `json:"id"`
	FirstFrame     int64  `json:"first_frame"`
	EndFrame       int64  `json:"end_frame"`
	DisplayStartNS int64  `json:"display_start_ns"`
	DisplayEndNS   int64  `json:"display_end_ns"`
	EncodedStartNS int64  `json:"encoded_start_ns"`
	EncodedEndNS   int64  `json:"encoded_end_ns"`
}

func runPlan(args []string) error {
	flags := flag.NewFlagSet("plan", flag.ContinueOnError)
	rateText := flags.String("rate", "", "exact output frame rate, e.g. 30000/1001")
	durationText := flags.String("duration", "", "media duration, e.g. 5m")
	srtPath := flags.String("srt", "", "input SRT")
	prerollText := flags.String("preroll", "0s", "encoder preroll")
	asJSON := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *rateText == "" || *durationText == "" || *srtPath == "" {
		return errors.New("plan requires -rate, -duration and -srt")
	}
	rate, err := subburn.ParseRate(*rateText)
	if err != nil {
		return err
	}
	duration, err := time.ParseDuration(*durationText)
	if err != nil {
		return err
	}
	preroll, err := time.ParseDuration(*prerollText)
	if err != nil {
		return err
	}
	file, err := os.Open(*srtPath)
	if err != nil {
		return err
	}
	cues, parseErr := subburn.ParseSRT(file)
	closeErr := file.Close()
	if parseErr != nil {
		return parseErr
	}
	if closeErr != nil {
		return closeErr
	}
	plan, err := subburn.BuildBurnPlan(rate, preroll, duration, cues)
	if err != nil {
		return err
	}
	if err := plan.Verify(subburn.ExactClock(rate), cues); err != nil {
		return err
	}
	if *asJSON {
		output := planJSON{
			Rate:          rate.String(),
			PrerollNS:     int64(preroll),
			DurationNS:    int64(duration),
			SubtitleQuery: "encoded_pts - preroll",
			Cues:          make([]placementJSON, len(plan.Cues)),
		}
		for index, placement := range plan.Cues {
			output.Cues[index] = placementJSON{
				ID:             placement.CueID,
				FirstFrame:     placement.FirstFrame,
				EndFrame:       placement.EndFrame,
				DisplayStartNS: int64(placement.DisplayStart),
				DisplayEndNS:   int64(placement.DisplayEnd),
				EncodedStartNS: int64(placement.EncodedStart),
				EncodedEndNS:   int64(placement.EncodedEnd),
			}
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}
	fmt.Printf("rate=%s preroll=%s duration=%s\n", rate, preroll, duration)
	for _, placement := range plan.Cues {
		fmt.Printf("%-10s frames=%d..%d display=%s..%s encoded=%s..%s\n",
			placement.CueID, placement.FirstFrame, placement.EndFrame-1,
			placement.DisplayStart, placement.DisplayEnd,
			placement.EncodedStart, placement.EncodedEnd)
	}
	return nil
}

func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func absolute(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func formatTime(value time.Duration) string {
	return value.Round(time.Millisecond).String()
}

func formatSigned(value time.Duration) string {
	if value > 0 {
		return "+" + value.Round(time.Millisecond).String()
	}
	return value.Round(time.Millisecond).String()
}

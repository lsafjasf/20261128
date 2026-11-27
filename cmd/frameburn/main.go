package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"frameburn/internal/audit"
	"frameburn/internal/sequence"
	"frameburn/internal/timeline"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "frameburn:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		args = []string{"demo"}
	}
	switch args[0] {
	case "generate":
		return runGenerate(args[1:])
	case "verify":
		return runVerify(args[1:])
	case "demo":
		return runDemo(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `usage:
  frameburn generate -out DIR [-config FILE] [-rate 23.976|29.97DF]
  frameburn verify -dir DIR [-report FILE]
  frameburn demo [-out DIR] [-width N] [-height N] [-frames N]

repeatable event flags for generate:
  -cut FRAME
  -black START:COUNT
  -pause START:COUNT`)
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("out", "out", "output directory")
	configPath := fs.String("config", "", "timeline JSON file")
	rate := fs.String("rate", "29.97DF", "23.976, 29.97DF, or 29.97ND")
	width := fs.Int("width", 480, "frame width")
	height := fs.Int("height", 270, "frame height")
	frames := fs.Int("frames", 150, "output frame count")
	quiet := fs.Bool("quiet", false, "suppress the summary")
	var events eventFlags
	fs.Func("cut", "edit point at a zero-based output frame", func(value string) error {
		return events.add(timeline.Cut, value)
	})
	fs.Func("black", "black run START:COUNT", func(value string) error {
		return events.add(timeline.Black, value)
	})
	fs.Func("pause", "freeze run START:COUNT", func(value string) error {
		return events.add(timeline.Pause, value)
	})
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := timeline.DefaultConfig(*rate)
	if *configPath != "" {
		data, err := os.ReadFile(*configPath)
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cfg); err != nil {
			return fmt.Errorf("%s: %w", *configPath, err)
		}
	} else {
		cfg = timeline.DefaultConfigSized(*rate, *width, *height, *frames)
	}
	if len(events.list) > 0 {
		cfg.Events = events.list
	}
	if err := generateOne(cfg, *out, *quiet); err != nil {
		return err
	}
	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "out", "sequence directory")
	reportPath := fs.String("report", "", "report path (default DIR/report.json)")
	quiet := fs.Bool("quiet", false, "suppress the summary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report, err := audit.Audit(*dir, audit.DefaultOptions())
	if err != nil {
		return err
	}
	if *reportPath == "" {
		*reportPath = filepath.Join(*dir, "report.json")
	}
	if err := writeJSON(*reportPath, report); err != nil {
		return err
	}
	if !*quiet {
		printReport(report)
		fmt.Println("report:", *reportPath)
	}
	if !report.OK {
		return fmt.Errorf("verification failed (%d error(s))", len(report.Errors))
	}
	return nil
}

func runDemo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("out", "out", "output directory")
	width := fs.Int("width", 320, "frame width")
	height := fs.Int("height", 180, "frame height")
	frames := fs.Int("frames", 90, "frames per rate")
	quiet := fs.Bool("quiet", false, "suppress the summary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	summary := demoSummary{OK: true, Results: make(map[string]*audit.Report)}
	var failures []string
	for _, rate := range []string{"23.976", "29.97DF"} {
		cfg := timeline.DefaultConfigSized(rate, *width, *height, *frames)
		dir := filepath.Join(*out, rate)
		report, err := generateAndAudit(cfg, dir, *quiet)
		if err != nil {
			failures = append(failures, rate+": "+err.Error())
			continue
		}
		summary.Results[rate] = report
		if !report.OK {
			summary.OK = false
			failures = append(failures, rate+": verification failed")
		}
	}
	if err := writeJSON(filepath.Join(*out, "demo-report.json"), summary); err != nil {
		return err
	}
	if !*quiet {
		fmt.Println("demo report:", filepath.Join(*out, "demo-report.json"))
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

type demoSummary struct {
	OK      bool                     `json:"ok"`
	Results map[string]*audit.Report `json:"results"`
}

type eventFlags struct {
	list []timeline.Event
}

func (f *eventFlags) add(kind timeline.EventKind, raw string) error {
	parts := strings.Split(raw, ":")
	start, err := strconv.Atoi(parts[0])
	if err != nil || start < 0 {
		return fmt.Errorf("invalid start %q", parts[0])
	}
	event := timeline.Event{Kind: kind, Start: start}
	if kind != timeline.Cut {
		if len(parts) != 2 {
			return fmt.Errorf("%s requires START:COUNT", kind)
		}
		count, err := strconv.Atoi(parts[1])
		if err != nil || count <= 0 {
			return fmt.Errorf("invalid count %q", parts[1])
		}
		event.Count = count
	} else if len(parts) != 1 {
		return fmt.Errorf("cut takes one frame number")
	}
	f.list = append(f.list, event)
	return nil
}

func generateOne(cfg timeline.Config, dir string, quiet bool) error {
	report, err := generateAndAudit(cfg, dir, quiet)
	if err != nil {
		return err
	}
	if !report.OK {
		return fmt.Errorf("verification failed (%d error(s))", len(report.Errors))
	}
	return nil
}

func generateAndAudit(cfg timeline.Config, dir string, quiet bool) (*audit.Report, error) {
	plan, err := timeline.Build(cfg)
	if err != nil {
		return nil, err
	}
	manifest, err := sequence.Write(plan, dir)
	if err != nil {
		return nil, err
	}
	report, err := audit.Audit(dir, audit.DefaultOptions())
	if err != nil {
		return nil, err
	}
	reportPath := filepath.Join(dir, "report.json")
	if err := writeJSON(reportPath, report); err != nil {
		return nil, err
	}
	if !quiet {
		fmt.Printf("%s: %d frames, content rect y=%d, report=%s\n",
			manifest.Rate, manifest.Frames, manifest.ContentRect.Y, reportPath)
		printReport(report)
	}
	return report, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func printReport(report *audit.Report) {
	fmt.Printf("  ok=%v labels=%v black=%v cuts=%v pauses=%v errors=%d\n",
		report.OK, report.Labels.OK, report.Black.OK,
		report.Cuts.OK, report.Pauses.OK, len(report.Errors))
	for _, message := range report.Errors {
		fmt.Println("  error:", message)
	}
}

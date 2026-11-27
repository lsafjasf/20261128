package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lsafjasf/20261128/channelmap"
)

const (
	exitValid    = 0
	exitRejected = 1
	exitUsage    = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type options struct {
	container string
	delivery  string
	outDir    string
	json      bool
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "validate" {
		args = args[1:]
	}
	opts, code, stop := parseOptions(args, stderr)
	if stop {
		return code
	}

	layout, err := channelmap.LoadContainer(opts.container)
	if err != nil {
		fmt.Fprintf(stderr, "input error: %v\n", err)
		return exitUsage
	}
	delivery, err := channelmap.LoadDelivery(opts.delivery)
	if err != nil {
		var rejected *channelmap.RejectionError
		if errors.As(err, &rejected) {
			return reportRejection(stdout, stderr, opts, err)
		}
		fmt.Fprintf(stderr, "input error: %v\n", err)
		return exitUsage
	}
	plan, err := channelmap.BuildPlan(layout, delivery)
	if err != nil {
		return reportRejection(stdout, stderr, opts, err)
	}
	evidence, err := channelmap.WriteEvidence(opts.outDir, plan)
	if err != nil {
		fmt.Fprintf(stderr, "evidence error: %v\n", err)
		return exitRejected
	}

	result := successResult{
		Valid:     true,
		Container: opts.container,
		Delivery:  opts.delivery,
		Identity:  plan.Identity,
		Channels:  len(plan.Mappings),
		Evidence: evidencePaths{
			Directory: opts.outDir,
			Mapping:   filepath.Join(opts.outDir, channelmap.MappingFileName),
			CSV:       filepath.Join(opts.outDir, channelmap.MappingCSVName),
			Readback:  filepath.Join(opts.outDir, channelmap.ReadbackFileName),
			SHA256:    evidence.MappingSHA256,
		},
	}
	if opts.json {
		encodeJSON(stdout, result)
		return exitValid
	}
	fmt.Fprintf(stdout, "VALID: %d channels, reordered=%t\n", result.Channels, !result.Identity)
	for _, row := range plan.Mappings {
		fmt.Fprintf(stdout, "  delivery %d %-4s <- source %d %-4s%s\n",
			row.Delivery.Position, row.Delivery.Label,
			row.Source.Position, row.Source.Label,
			reorderedSuffix(row.Reordered))
	}
	fmt.Fprintf(stdout, "mapping:  %s\n", result.Evidence.Mapping)
	fmt.Fprintf(stdout, "csv:      %s\n", result.Evidence.CSV)
	fmt.Fprintf(stdout, "readback: %s (verified)\n", result.Evidence.Readback)
	fmt.Fprintf(stdout, "sha256:   %s\n", result.Evidence.SHA256)
	return exitValid
}

func parseOptions(args []string, stderr io.Writer) (options, int, bool) {
	opts := options{outDir: "evidence"}
	flags := flag.NewFlagSet("audiomap-validator", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.container, "container", "", "WAV/RF64 file or JSON container descriptor")
	flags.StringVar(&opts.delivery, "delivery", "", "external delivery channel map (JSON)")
	flags.StringVar(&opts.outDir, "out", opts.outDir, "evidence output directory")
	flags.BoolVar(&opts.json, "json", false, "print a JSON result")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: audiomap-validator validate --container <file> --delivery <file> [--out dir] [--json]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, exitValid, true
		}
		return options{}, exitUsage, true
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		flags.Usage()
		return options{}, exitUsage, true
	}
	if opts.container == "" || opts.delivery == "" {
		fmt.Fprintln(stderr, "both --container and --delivery are required")
		flags.Usage()
		return options{}, exitUsage, true
	}
	return opts, exitValid, false
}

type successResult struct {
	Valid     bool          `json:"valid"`
	Container string        `json:"container"`
	Delivery  string        `json:"delivery"`
	Identity  bool          `json:"identity"`
	Channels  int           `json:"channels"`
	Evidence  evidencePaths `json:"evidence"`
}

type evidencePaths struct {
	Directory string `json:"directory"`
	Mapping   string `json:"mapping"`
	CSV       string `json:"csv"`
	Readback  string `json:"readback"`
	SHA256    string `json:"mapping_sha256"`
}

type rejectionResult struct {
	Valid     bool               `json:"valid"`
	Container string             `json:"container"`
	Delivery  string             `json:"delivery"`
	Error     string             `json:"error"`
	Issues    []channelmap.Issue `json:"issues,omitempty"`
	Evidence  string             `json:"rejection_file,omitempty"`
}

func reportRejection(stdout, stderr io.Writer, opts options, err error) int {
	result := rejectionResult{
		Valid:     false,
		Container: opts.container,
		Delivery:  opts.delivery,
		Error:     err.Error(),
	}
	var rejected *channelmap.RejectionError
	if errors.As(err, &rejected) {
		result.Issues = rejected.Issues
	}
	if mkdirErr := os.MkdirAll(opts.outDir, 0o755); mkdirErr == nil {
		path := filepath.Join(opts.outDir, "rejection.json")
		if data, marshalErr := json.MarshalIndent(result, "", "  "); marshalErr == nil {
			if writeErr := os.WriteFile(path, append(data, '\n'), 0o644); writeErr == nil {
				result.Evidence = path
			}
		}
	}
	if opts.json {
		encodeJSON(stdout, result)
	} else {
		fmt.Fprintf(stdout, "REJECTED: %s\n", err)
		if result.Evidence != "" {
			fmt.Fprintf(stdout, "details: %s\n", result.Evidence)
		}
	}
	if len(result.Issues) == 0 {
		fmt.Fprintf(stderr, "mapping rejected: %v\n", err)
	}
	return exitRejected
}

func encodeJSON(writer io.Writer, value any) {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func reorderedSuffix(reordered bool) string {
	if reordered {
		return "  [REORDERED]"
	}
	return ""
}

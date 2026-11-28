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

	"github.com/lsafjasf/20261128/internal/ingest"
	"github.com/lsafjasf/20261128/internal/ltc"
)

type sourceSpec struct {
	label string
	path  string
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "ltc-timeline:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("ltc-timeline", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fps := fs.Int("fps", 0, "nominal frame rate: 24, 25, 30, or 0 to infer")
	order := fs.String("bit-order", "auto", "packed-byte bit order: auto, msb, or lsb")
	inputFormat := fs.String("input-format", "auto", "input format: auto, text, or binary")
	compact := fs.Bool("compact", false, "emit compact JSON")
	output := fs.String("output", "-", "write JSON to this path, or - for stdout")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: ltc-timeline [options] [LABEL=]FILE [[LABEL=]FILE ...]")
		fmt.Fprintln(stderr, "       use FILE '-' to read one source from stdin")
		fs.PrintDefaults()
	}
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return errors.New("at least one LTC input is required")
	}
	if *fps != 0 && *fps != 24 && *fps != 25 && *fps != 30 {
		return fmt.Errorf("unsupported --fps %d (use 24, 25, 30, or 0)", *fps)
	}
	bitOrder := ltc.BitOrder(strings.ToLower(*order))
	if bitOrder != ltc.BitOrderAuto && bitOrder != ltc.BitOrderMSB && bitOrder != ltc.BitOrderLSB {
		return fmt.Errorf("unsupported --bit-order %q", *order)
	}
	format := ingest.Format(strings.ToLower(*inputFormat))
	if format != ingest.FormatAuto && format != ingest.FormatText && format != ingest.FormatBinary {
		return fmt.Errorf("unsupported --input-format %q", *inputFormat)
	}

	specs, err := parseSources(fs.Args())
	if err != nil {
		return err
	}
	opts := ingest.Options{NominalFPS: *fps, BitOrder: bitOrder, Format: format}
	timelines := make([]ltc.Timeline, 0, len(specs))
	seenLabels := make(map[string]bool)
	stdinUsed := false
	for _, spec := range specs {
		if seenLabels[spec.label] {
			return fmt.Errorf("duplicate source label %q", spec.label)
		}
		seenLabels[spec.label] = true

		var result ingest.Result
		if spec.path == "-" {
			if stdinUsed {
				return errors.New("stdin can only be used for one source")
			}
			stdinUsed = true
			data, readErr := io.ReadAll(stdin)
			if readErr != nil {
				return fmt.Errorf("read stdin: %w", readErr)
			}
			result, err = ingest.ParseBytes(data, opts)
		} else {
			result, err = ingest.ReadFile(spec.path, opts)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", spec.label, err)
		}
		t := ltc.Analyze(spec.label, result.Frames, result.NominalFPS)
		t.InputFormat = string(result.Format)
		t.BitOrder = string(result.BitOrder)
		t.FPSInferred = result.InferredFPS
		timelines = append(timelines, t)
	}

	analysis := ltc.BuildAnalysis(timelines)
	var out io.Writer = stdout
	var file *os.File
	if *output != "-" {
		file, err = os.Create(*output)
		if err != nil {
			return err
		}
		defer file.Close()
		out = file
	}
	enc := json.NewEncoder(out)
	if !*compact {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(analysis); err != nil {
		return err
	}
	return nil
}

// The standard flag package stops at the first positional argument. Permit
// options and sources to be interspersed, which is friendlier for shell use.
func reorderArgs(args []string) []string {
	flags := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		if !strings.Contains(arg, "=") && flagNeedsValue(arg) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}

func flagNeedsValue(arg string) bool {
	name := strings.TrimLeft(arg, "-")
	switch name {
	case "fps", "bit-order", "input-format", "output":
		return true
	default:
		return false
	}
}

func parseSources(args []string) ([]sourceSpec, error) {
	var specs []sourceSpec
	used := make(map[string]bool)
	for _, arg := range args {
		label, path := splitSource(arg)
		if path == "" {
			return nil, fmt.Errorf("empty input path in %q", arg)
		}
		if used[path] {
			return nil, fmt.Errorf("input path %q specified more than once", path)
		}
		used[path] = true
		if label == "" {
			label = defaultLabel(path)
		}
		specs = append(specs, sourceSpec{label: label, path: path})
	}
	return specs, nil
}

func splitSource(arg string) (string, string) {
	arg = strings.TrimSpace(arg)
	if i := strings.IndexByte(arg, '='); i > 0 {
		left, right := arg[:i], arg[i+1:]
		if right != "" && !strings.ContainsAny(left, `/\`) {
			if _, err := os.Stat(right); err == nil || right == "-" {
				return left, right
			}
		}
	}
	return "", arg
}

func defaultLabel(path string) string {
	if path == "-" {
		return "stdin"
	}
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)
	if base == "" {
		return "input"
	}
	return base
}

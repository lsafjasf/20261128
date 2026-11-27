package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"interlace-diag/internal/analyze"
	"interlace-diag/internal/output"
	"interlace-diag/internal/video"
)

const version = "1.0.0"

type config struct {
	input        string
	inputFormat  string
	size         string
	pixFmt       string
	fps          float64
	fieldOrder   string
	maxShiftX    int
	maxShiftY    int
	outputFormat string
	outputPath   string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	source, format, closeInput, err := openSource(cfg)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	defer closeInput()

	declared := resolveDeclaredOrder(cfg.fieldOrder, format)
	analyzer := analyze.NewAnalyzer(analyze.Options{
		MaxShiftX:     cfg.maxShiftX,
		MaxShiftY:     cfg.maxShiftY,
		DeclaredOrder: declared,
	})
	out, closeOutput, err := openOutput(cfg.outputPath, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	defer closeOutput()
	writer, err := output.NewWriter(out, cfg.outputFormat, analyze.Document{
		Type:      "document",
		Input:     cfg.input,
		Width:     format.Width,
		Height:    format.Height,
		FrameRate: format.FrameRate(),
	})
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}

	for {
		frame, err := source.Next()
		if errors.Is(err, video.ErrEndOfStream) {
			break
		}
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		rec, err := analyzer.Push(frame)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		if rec != nil {
			if err := writer.WriteFrame(*rec); err != nil {
				fmt.Fprintln(stderr, "error: write report:", err)
				return 1
			}
		}
	}
	if rec := analyzer.Flush(); rec != nil {
		if err := writer.WriteFrame(*rec); err != nil {
			fmt.Fprintln(stderr, "error: write report:", err)
			return 1
		}
	}
	if err := writer.Close(analyzer.Summary()); err != nil {
		fmt.Fprintln(stderr, "error: close report:", err)
		return 1
	}
	return 0
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("interlace-diag", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.input, "input", "-", "input file, or - for stdin")
	fs.StringVar(&cfg.input, "i", "-", "alias for -input")
	fs.StringVar(&cfg.inputFormat, "format", "auto", "input format: auto, y4m or raw")
	fs.StringVar(&cfg.size, "size", "", "raw frame size, for example 1920x1080")
	fs.StringVar(&cfg.pixFmt, "pix-fmt", "yuv420p", "raw pixel layout: yuv420p, yuv422p, yuv444p or gray")
	fs.Float64Var(&cfg.fps, "fps", 25, "raw input frame rate")
	fs.StringVar(&cfg.fieldOrder, "field-order", "auto", "field order hint: auto, tff or bff")
	fs.IntVar(&cfg.maxShiftX, "max-shift-x", 0, "maximum horizontal field motion in pixels (0 = automatic)")
	fs.IntVar(&cfg.maxShiftY, "max-shift-y", 0, "maximum vertical field motion in pixels (0 = automatic)")
	fs.StringVar(&cfg.outputFormat, "output-format", "jsonl", "report format: jsonl, json or text")
	fs.StringVar(&cfg.outputPath, "output", "-", "report file, or - for stdout")
	showVersion := fs.Bool("version", false, "print version")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "interlace-diag: per-frame interlacing artifact evidence (Go, zero dependencies)")
		fmt.Fprintln(stderr, "\nUsage: interlace-diag -input FILE [-format y4m|raw] [options]")
		fmt.Fprintln(stderr, "\nOptions:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if *showVersion {
		fmt.Fprintln(stderr, "interlace-diag", version)
		return cfg, flag.ErrHelp
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected positional argument %q", fs.Arg(0))
	}
	switch strings.ToLower(cfg.fieldOrder) {
	case "auto", "tff", "bff":
	default:
		return cfg, fmt.Errorf("-field-order must be auto, tff or bff")
	}
	return cfg, nil
}

func openSource(cfg config) (video.Source, video.Format, func(), error) {
	var (
		in   io.Reader = os.Stdin
		file *os.File
	)
	if cfg.input != "-" {
		f, err := os.Open(cfg.input)
		if err != nil {
			return nil, video.Format{}, func() {}, err
		}
		file, in = f, f
	}
	closeFn := func() {
		if file != nil {
			_ = file.Close()
		}
	}
	formatName := strings.ToLower(cfg.inputFormat)
	if formatName == "auto" {
		switch strings.ToLower(filepath.Ext(cfg.input)) {
		case ".y4m":
			formatName = "y4m"
		default:
			if cfg.size != "" {
				formatName = "raw"
			}
		}
	}
	var (
		source video.Source
		format video.Format
		err    error
	)
	switch formatName {
	case "y4m":
		var r *video.Y4MReader
		r, err = video.NewY4MReader(in)
		if err == nil {
			source, format = r, r.Format()
		}
	case "raw", "yuv", "yuv420p":
		w, h, parseErr := parseSize(cfg.size)
		if parseErr != nil {
			err = parseErr
			break
		}
		var r *video.RawReader
		r, err = video.NewRawReader(in, w, h, cfg.fps, cfg.pixFmt)
		if err == nil {
			source, format = r, r.Format()
		}
	default:
		err = fmt.Errorf("cannot infer input format; use -format y4m or -format raw with -size WxH")
	}
	if err != nil {
		closeFn()
		return nil, video.Format{}, func() {}, err
	}
	return source, format, closeFn, nil
}

func parseSize(value string) (int, int, error) {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return r == 'x' || r == '*' })
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("-size must be WIDTHxHEIGHT, got %q", value)
	}
	w, errW := strconv.Atoi(parts[0])
	h, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("invalid -size %q", value)
	}
	return w, h, nil
}

func resolveDeclaredOrder(flagValue string, f video.Format) string {
	if flagValue != "auto" {
		return flagValue
	}
	switch f.Interlace {
	case "t", "tt", "tff":
		return "tff"
	case "b", "bb", "bff":
		return "bff"
	default:
		return ""
	}
}

func openOutput(path string, stdout io.Writer) (io.Writer, func(), error) {
	if path == "-" {
		return stdout, func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, func() {}, err
	}
	return f, func() { _ = f.Close() }, nil
}

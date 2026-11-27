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

	"ghostdiag"
)

const version = "1.0.0"

type config struct {
	original     string
	denoised     string
	inputFormat  string
	size         string
	pixelFormat  string
	frameRate    float64
	trajectory   string
	outputFormat string
	output       string
	failOnGhost  bool
	showVersion  bool
	options      ghostdiag.Options
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
	if cfg.showVersion {
		fmt.Fprintln(stdout, "ghostdiag", version)
		return 0
	}
	original, closeOriginal, err := openSource(cfg, cfg.original)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	defer closeOriginal()
	denoised, closeDenoised, err := openSource(cfg, cfg.denoised)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	defer closeDenoised()
	trajectory := ghostdiag.Trajectory{}
	if cfg.trajectory != "" {
		trajectory, err = ghostdiag.LoadTrajectory(cfg.trajectory)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 2
		}
	}
	report, err := ghostdiag.Analyze(original, denoised, trajectory, cfg.options)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	out, closeOutput, err := openOutput(cfg.output, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	defer closeOutput()
	switch strings.ToLower(cfg.outputFormat) {
	case "json":
		err = ghostdiag.WriteJSON(out, report)
	case "text":
		err = ghostdiag.WriteText(out, report)
	default:
		err = fmt.Errorf("unsupported output format %q", cfg.outputFormat)
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if cfg.failOnGhost && report.FirstGhost != nil {
		return 2
	}
	return 0
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	cfg := config{options: ghostdiag.DefaultOptions()}
	fs := flag.NewFlagSet("ghostdiag", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.original, "original", "", "original/reference sequence")
	fs.StringVar(&cfg.original, "o", "", "alias for -original")
	fs.StringVar(&cfg.denoised, "denoised", "", "denoised sequence")
	fs.StringVar(&cfg.denoised, "d", "", "alias for -denoised")
	fs.StringVar(&cfg.inputFormat, "format", "auto", "input format: auto, y4m, raw or images")
	fs.StringVar(&cfg.size, "size", "", "raw frame size, for example 1920x1080")
	fs.StringVar(&cfg.pixelFormat, "pix-fmt", "yuv420p", "raw pixel layout")
	fs.Float64Var(&cfg.frameRate, "fps", 25, "frame rate for raw/images")
	fs.StringVar(&cfg.trajectory, "trajectory", "", "optional JSON motion trajectory")
	fs.StringVar(&cfg.outputFormat, "output-format", "json", "report format: json or text")
	fs.StringVar(&cfg.output, "output", "-", "report file, or - for stdout")
	fs.Float64Var(&cfg.options.GhostThreshold, "ghost-threshold", cfg.options.GhostThreshold, "temporal lag score threshold")
	fs.IntVar(&cfg.options.NoiseThreshold, "noise-threshold", cfg.options.NoiseThreshold, "minimum denoise residual")
	fs.IntVar(&cfg.options.MotionThreshold, "motion-threshold", cfg.options.MotionThreshold, "minimum source temporal change")
	fs.IntVar(&cfg.options.HistoryFrames, "history", cfg.options.HistoryFrames, "frames of temporal history")
	fs.IntVar(&cfg.options.MinArea, "min-area", cfg.options.MinArea, "minimum artifact component area")
	fs.IntVar(&cfg.options.BlockSize, "block-size", cfg.options.BlockSize, "coding grid size")
	fs.Float64Var(&cfg.options.BlockThreshold, "block-threshold", cfg.options.BlockThreshold, "coding block score threshold")
	fs.Float64Var(&cfg.options.BlurWidthThreshold, "blur-width", cfg.options.BlurWidthThreshold, "source motion blur width in pixels")
	fs.BoolVar(&cfg.failOnGhost, "fail-on-ghost", false, "exit 2 when temporal ghosting is found")
	fs.BoolVar(&cfg.showVersion, "version", false, "print version")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "ghostdiag: compare original and denoised frames (Go, zero dependencies)")
		fmt.Fprintln(stderr, "\nUsage: ghostdiag -original ORIG -denoised DEN [options]")
		fmt.Fprintln(stderr, "\nOptions:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected positional argument %q", fs.Arg(0))
	}
	if cfg.showVersion {
		return cfg, nil
	}
	if cfg.original == "" || cfg.denoised == "" {
		return cfg, errors.New("-original and -denoised are required")
	}
	switch strings.ToLower(cfg.inputFormat) {
	case "auto", "y4m", "raw", "images":
	default:
		return cfg, fmt.Errorf("unsupported input format %q", cfg.inputFormat)
	}
	return cfg, nil
}

func openSource(cfg config, path string) (ghostdiag.Source, func(), error) {
	format := strings.ToLower(cfg.inputFormat)
	if format == "auto" {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			format = "images"
		} else if strings.EqualFold(filepath.Ext(path), ".y4m") {
			format = "y4m"
		} else if cfg.size != "" {
			format = "raw"
		} else {
			return nil, func() {}, errors.New("cannot infer format; use -format y4m, raw (with -size) or images")
		}
	}
	if format == "images" {
		source, err := ghostdiag.NewImageSource(path, cfg.frameRate)
		if err != nil {
			return nil, func() {}, err
		}
		return source, func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	closeFn := func() { _ = file.Close() }
	if format == "y4m" {
		source, err := ghostdiag.NewY4MSource(file)
		if err != nil {
			closeFn()
			return nil, func() {}, err
		}
		return source, closeFn, nil
	}
	width, height, err := parseSize(cfg.size)
	if err != nil {
		closeFn()
		return nil, func() {}, err
	}
	source, err := ghostdiag.NewRawSource(file, ghostdiag.RawConfig{
		Width: width, Height: height, FrameRate: cfg.frameRate, PixelFormat: cfg.pixelFormat,
	})
	if err != nil {
		closeFn()
		return nil, func() {}, err
	}
	return source, closeFn, nil
}

func parseSize(value string) (int, int, error) {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return r == 'x' || r == '*' })
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("-size must be WIDTHxHEIGHT, got %q", value)
	}
	width, errW := strconv.Atoi(parts[0])
	height, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("invalid -size %q", value)
	}
	return width, height, nil
}

func openOutput(path string, stdout io.Writer) (io.Writer, func(), error) {
	if path == "-" {
		return stdout, func() {}, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, func() {}, err
	}
	return file, func() { _ = file.Close() }, nil
}

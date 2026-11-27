package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"yuvdiag"
)

type config struct {
	input           string
	reference       string
	width           int
	height          int
	bitDepth        int
	sampleFormat    string
	rawRange        string
	matrix          string
	chroma          string
	rgbDepth        int
	yStride         int
	cStride         int
	frameIndex      int
	maxViolations   int
	jsonOut         bool
	reportPath      string
	ppmPath         string
	failOnViolation bool
}

func main() {
	code, err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "yuvdiag:", err)
	}
	os.Exit(code)
}

func run(args []string, stdout, stderr io.Writer) (int, error) {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		return 2, err
	}
	format := yuv.SampleFormat(cfg.sampleFormat)
	layout := yuv.NewLayout(cfg.width, cfg.height, cfg.bitDepth, format)
	layout.YStride = cfg.yStride
	layout.CStride = cfg.cStride
	decoded, err := loadFrame(cfg.input, layout, cfg.frameIndex)
	if err != nil {
		return 1, err
	}
	var reference *yuv.Frame
	if cfg.reference != "" {
		reference, err = loadFrame(cfg.reference, layout, cfg.frameIndex)
		if err != nil {
			return 1, fmt.Errorf("reference: %w", err)
		}
	}
	rng, err := yuv.ParseRange(cfg.rawRange)
	if err != nil {
		return 2, err
	}
	matrix, err := yuv.ParseMatrix(cfg.matrix)
	if err != nil {
		return 2, err
	}
	chroma, err := yuv.ParseChromaLocation(cfg.chroma)
	if err != nil {
		return 2, err
	}
	opts := yuv.ConvertOptions{
		Matrix: matrix, Range: rng, Chroma: chroma, RGBBitDepth: cfg.rgbDepth,
	}
	report, err := yuv.BuildReport(cfg.input, cfg.reference, decoded, reference,
		opts, cfg.maxViolations)
	if err != nil {
		return 1, err
	}
	if cfg.jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return 1, err
		}
	} else {
		renderText(stdout, report)
	}
	if cfg.reportPath != "" {
		if err := writeJSON(cfg.reportPath, report); err != nil {
			return 1, err
		}
	}
	if cfg.ppmPath != "" {
		rgb, _, err := yuv.ConvertFrame(decoded, opts, 0)
		if err != nil {
			return 1, err
		}
		if err := writePPM(cfg.ppmPath, rgb); err != nil {
			return 1, err
		}
	}
	if cfg.failOnViolation && (report.Summary.TrueOutOfRangePixels > 0 ||
		report.Summary.DisplayClippedPixels > 0) {
		return 2, nil
	}
	return 0, nil
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	cfg := config{}
	fs := flag.NewFlagSet("yuvdiag", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.input, "input", "", "decoded raw 4:2:0 YUV file")
	fs.StringVar(&cfg.reference, "reference", "", "optional pre-decode/source raw YUV file")
	fs.IntVar(&cfg.width, "width", 0, "frame width")
	fs.IntVar(&cfg.height, "height", 0, "frame height")
	fs.IntVar(&cfg.bitDepth, "bit-depth", 10, "YUV bit depth")
	fs.StringVar(&cfg.sampleFormat, "pix-fmt", "yuv420p10le", "yuv420p10le or p010le")
	fs.StringVar(&cfg.rawRange, "range", "limited", "limited or full")
	fs.StringVar(&cfg.matrix, "matrix", "bt709", "bt601, bt709 or bt2020")
	fs.StringVar(&cfg.chroma, "chroma", "left", "left, center, top or topleft")
	fs.IntVar(&cfg.rgbDepth, "rgb-depth", 8, "display RGB bit depth")
	fs.IntVar(&cfg.yStride, "y-stride", 0, "Y samples per row (0 = width)")
	fs.IntVar(&cfg.cStride, "c-stride", 0, "chroma samples per row (0 = ceil(width/2))")
	fs.IntVar(&cfg.frameIndex, "frame", 0, "zero-based frame index")
	fs.IntVar(&cfg.maxViolations, "max-violations", 128, "maximum sampled violations in report")
	fs.BoolVar(&cfg.jsonOut, "json", false, "write JSON report to stdout")
	fs.StringVar(&cfg.reportPath, "report", "", "also write JSON report to this file")
	fs.StringVar(&cfg.ppmPath, "ppm", "", "write corrected RGB frame as binary PPM")
	fs.BoolVar(&cfg.failOnViolation, "fail-on-violation", false, "exit 2 on true overflow or display clipping")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: yuvdiag -input decoded.yuv -width W -height H [options] [input.yuv]")
		fmt.Fprintln(stderr, "Audit 10-bit 4:2:0 YUV before conversion and distinguish code overflow from display clipping.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.input == "" && fs.NArg() == 1 {
		cfg.input = fs.Arg(0)
	} else if fs.NArg() > 0 {
		return config{}, errors.New("unexpected positional arguments")
	}
	if cfg.input == "" {
		return config{}, errors.New("-input is required")
	}
	if cfg.width <= 0 || cfg.height <= 0 {
		return config{}, errors.New("-width and -height must be positive")
	}
	if cfg.yStride == 0 {
		cfg.yStride = cfg.width
	}
	if cfg.cStride == 0 {
		cfg.cStride = (cfg.width + 1) / 2
	}
	if cfg.frameIndex < 0 {
		return config{}, errors.New("-frame must be non-negative")
	}
	if cfg.maxViolations < 0 {
		return config{}, errors.New("-max-violations must be non-negative")
	}
	return cfg, nil
}

func loadFrame(path string, layout yuv.Layout, frameIndex int) (*yuv.Frame, error) {
	size, err := layout.FrameSize()
	if err != nil {
		return nil, err
	}
	if int64(frameIndex) > (1<<63-1)/size {
		return nil, fmt.Errorf("frame index %d overflows file offset", frameIndex)
	}
	offset := int64(frameIndex) * size
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < offset+size {
		return nil, fmt.Errorf("%s: frame %d ends at %d, file has %d bytes",
			path, frameIndex, offset+size, info.Size())
	}
	frame, err := yuv.ReadFrameAt(file, offset, layout)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return frame, nil
}

func writeJSON(path string, report *yuv.Report) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

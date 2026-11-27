package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"resampleforensics/internal/analysis"
	"resampleforensics/internal/report"
	"resampleforensics/internal/wavio"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "resampleforensics:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("resampleforensics", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	srcPath := fs.String("src", "", "native-rate WAV source (for example 44.1 kHz)")
	dstPath := fs.String("dst", "", "converted WAV destination (for example 48 kHz)")
	srcChannel := fs.Int("source-channel", -1, "zero-based source channel; -1 downmixes")
	dstChannel := fs.Int("destination-channel", -1, "zero-based destination channel; -1 downmixes")
	cutoff := fs.Float64("cutoff", 0, "reference passband edge in Hz; 0 chooses 95% of the lower Nyquist frequency")
	halfWidth := fs.Int("reference-half-width", 0, "reference sinc half-width in source samples; 0 chooses 64")
	fftSize := fs.Int("fft", 0, "Welch FFT size (power of two); 0 chooses 32768")
	maxTransients := fs.Int("max-transients", 0, "maximum transients to report; 0 chooses 12")
	boundaryWindow := fs.Float64("boundary-window-ms", 0, "edge comparison window in ms; 0 chooses 50")
	minCoherence := fs.Float64("min-coherence", 0, "complex-transfer coherence gate; 0 chooses 0.85")
	format := fs.String("format", "text", "output format: text or json")
	outPath := fs.String("out", "", "output path; empty writes to stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *srcPath == "" || *dstPath == "" {
		fs.Usage()
		return fmt.Errorf("both -src and -dst are required")
	}
	if *format != "text" && *format != "json" {
		return fmt.Errorf("-format must be text or json")
	}
	src, srcInfo, err := wavio.ReadMono(*srcPath, *srcChannel)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	dst, dstInfo, err := wavio.ReadMono(*dstPath, *dstChannel)
	if err != nil {
		return fmt.Errorf("read destination: %w", err)
	}
	opts := analysis.Options{
		CutoffHz:           *cutoff,
		ReferenceHalfWidth: *halfWidth,
		FFTSize:            *fftSize,
		MaxTransients:      *maxTransients,
		BoundaryWindowMs:   *boundaryWindow,
		MinCoherence:       *minCoherence,
	}
	result, err := analysis.Analyze(src, srcInfo.SampleRate, dst, dstInfo.SampleRate, opts)
	if err != nil {
		return err
	}
	out := os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	if *format == "json" {
		encoded, err := report.JSON(result)
		if err != nil {
			return err
		}
		if _, err := out.Write(append(encoded, '\n')); err != nil {
			return err
		}
		return nil
	}
	return report.Text(out, result)
}

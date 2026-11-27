package analysis

import (
	"math"
	"strings"
	"testing"

	"resampleforensics/internal/dsp"
)

func syntheticPercussive(rate int, seconds float64) []float64 {
	n := int(float64(rate) * seconds)
	x := make([]float64, n)
	for i := range x {
		t := float64(i) / float64(rate)
		x[i] = 0.040*math.Sin(2*math.Pi*220*t+0.7) + 0.020*math.Cos(2*math.Pi*333*t)
	}
	for _, at := range []float64{0.30, 0.72, 1.10, 1.48, 1.82} {
		start := int(at * float64(rate))
		for j := 0; j < int(0.06*float64(rate)) && start+j < n; j++ {
			t := float64(j) / float64(rate)
			env := math.Exp(-t / 0.009)
			v := 0.55 * env * (math.Sin(2*math.Pi*4200*t) + 0.35*math.Sin(2*math.Pi*8700*t))
			if j == 0 {
				v += 0.8
			}
			x[start+j] += v
		}
	}
	return x
}

func TestAnalyzeExactReferenceDoesNotInventWidening(t *testing.T) {
	const srcRate, dstRate = 44100, 48000
	src := syntheticPercussive(srcRate, 2.2)
	opts := DefaultOptions(srcRate, dstRate)
	rs, err := dsp.NewSincResampler(srcRate, dstRate, opts.CutoffHz, opts.ReferenceHalfWidth)
	if err != nil {
		t.Fatal(err)
	}
	dst := rs.Resample(src, dsp.BoundaryReflect)
	report, err := Analyze(src, srcRate, dst, dstRate, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Transients.DetectedCount < 3 {
		t.Fatalf("detected %d transients: %+v", report.Transients.DetectedCount, report.Transients)
	}
	if report.Transients.MedianWidthRatio < 0.98 || report.Transients.MedianWidthRatio > 1.02 {
		t.Fatalf("identical conversion width ratio %.4f, events=%+v", report.Transients.MedianWidthRatio, report.Transients.Events)
	}
	if report.Attribution.Classification == "resampler" {
		t.Fatalf("false resampler attribution: %+v", report.Attribution)
	}
	if math.Abs(report.Alignment.LagSamples) > 0.75 {
		t.Fatalf("unexpected alignment lag %.3f", report.Alignment.LagSamples)
	}
}

func TestAnalyzeDetectsLowpassAndZeroBoundary(t *testing.T) {
	const srcRate, dstRate = 44100, 48000
	src := syntheticPercussive(srcRate, 2.2)
	rs, err := dsp.NewSincResampler(srcRate, dstRate, 3000, 8)
	if err != nil {
		t.Fatal(err)
	}
	converted := rs.Resample(src, dsp.BoundaryZero)
	dst := append(make([]float64, 37), converted...)
	report, err := Analyze(src, srcRate, dst, dstRate, DefaultOptions(srcRate, dstRate))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Transients.ConverterWidening {
		t.Fatalf("lowpass widening not detected, width=%.4f rise=%.4f events=%+v",
			report.Transients.MedianWidthRatio, report.Transients.MedianRiseRatio, report.Transients.Events)
	}
	if report.Attribution.Classification != "resampler" {
		t.Fatalf("classification=%q evidence=%v", report.Attribution.Classification, report.Attribution.Evidence)
	}
	if report.Boundary.Indeterminate || report.Boundary.BestMode != "zero" {
		t.Fatalf("boundary mode=%q indeterminate=%v scores=%+v", report.Boundary.BestMode, report.Boundary.Indeterminate, report.Boundary.Scores)
	}
	if math.Abs(report.Alignment.LagSamples-37) > 2 {
		t.Fatalf("alignment lag %.3f, want about 37", report.Alignment.LagSamples)
	}
}

func TestGroupDelayIsLatencyReferenced(t *testing.T) {
	const srcRate, dstRate = 44100, 48000
	src := syntheticPercussive(srcRate, 1.8)
	opts := DefaultOptions(srcRate, dstRate)
	rs, err := dsp.NewSincResampler(srcRate, dstRate, opts.CutoffHz, opts.ReferenceHalfWidth)
	if err != nil {
		t.Fatal(err)
	}
	ref := rs.Resample(src, dsp.BoundaryReflect)
	dst := append(make([]float64, 100), ref...)
	report, err := Analyze(src, srcRate, dst, dstRate, opts)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(report.Alignment.LagSamples-100) > 0.75 {
		t.Fatalf("alignment %.3f, want 100", report.Alignment.LagSamples)
	}
	wantMS := 100.0 / dstRate * 1000
	for _, band := range report.GroupDelay.Bands {
		if math.Abs(band.TotalDelayMS-wantMS) > 0.15 {
			t.Fatalf("%s total group delay %.4f ms, want %.4f", band.Name, band.TotalDelayMS, wantMS)
		}
		if math.Abs(band.GroupDelayMS) > 0.15 {
			t.Fatalf("%s residual group delay %.4f ms", band.Name, band.GroupDelayMS)
		}
	}
	if report.GroupDelay.AggregateSpectrumUsed {
		t.Fatal("aggregate spectral similarity must not be used")
	}
}

func TestBoundarySearchDistinguishesFilterLength(t *testing.T) {
	const srcRate, dstRate = 44100, 48000
	src := syntheticPercussive(srcRate, 1.8)
	opts := DefaultOptions(srcRate, dstRate)
	shortRS, err := dsp.NewSincResampler(srcRate, dstRate, 10000, 6)
	if err != nil {
		t.Fatal(err)
	}
	longRS, err := dsp.NewSincResampler(srcRate, dstRate, 10000, 64)
	if err != nil {
		t.Fatal(err)
	}
	shortDst := shortRS.Resample(src, dsp.BoundaryReflect)
	longDst := longRS.Resample(src, dsp.BoundaryReflect)
	shortReport, err := Analyze(src, srcRate, shortDst, dstRate, opts)
	if err != nil {
		t.Fatal(err)
	}
	longReport, err := Analyze(src, srcRate, longDst, dstRate, opts)
	if err != nil {
		t.Fatal(err)
	}
	if longReport.Boundary.BestHalfWidth <= shortReport.Boundary.BestHalfWidth*2 {
		t.Fatalf("long best half %d, short %d; impulse long/short %.3f/%.3f ms",
			longReport.Boundary.BestHalfWidth, shortReport.Boundary.BestHalfWidth,
			longReport.Impulse.Support60DBMS, shortReport.Impulse.Support60DBMS)
	}
	if longReport.Impulse.Support60DBMS <= shortReport.Impulse.Support60DBMS*1.2 {
		t.Fatalf("impulse support long=%.3f ms short=%.3f ms",
			longReport.Impulse.Support60DBMS, shortReport.Impulse.Support60DBMS)
	}
}

func TestAnalyzeShortFileFallsBackSafely(t *testing.T) {
	const srcRate, dstRate = 44100, 48000
	src := syntheticPercussive(srcRate, 0.30)
	opts := DefaultOptions(srcRate, dstRate)
	rs, err := dsp.NewSincResampler(srcRate, dstRate, opts.CutoffHz, 32)
	if err != nil {
		t.Fatal(err)
	}
	dst := rs.Resample(src, dsp.BoundaryReflect)
	report, err := Analyze(src, srcRate, dst, dstRate, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Conversion != "44100 Hz -> 48000 Hz" {
		t.Fatalf("conversion %q", report.Conversion)
	}
	found := false
	for _, warning := range report.Warnings {
		if strings.Contains(warning, "FFT size reduced") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected short-file warning, got %v", report.Warnings)
	}
}

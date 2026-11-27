package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"resampleforensics/internal/analysis"
	"resampleforensics/internal/dsp"
	"resampleforensics/internal/wavio"
)

func TestRunJSONAndText(t *testing.T) {
	dir := t.TempDir()
	const srcRate, dstRate = 44100, 48000
	src := make([]float64, srcRate)
	for i := range src {
		src[i] = 0.03 * math.Sin(2*math.Pi*220*float64(i)/srcRate)
	}
	for _, at := range []float64{0.20, 0.50, 0.80} {
		start := int(at * srcRate)
		for j := 0; j < srcRate/50 && start+j < len(src); j++ {
			tt := float64(j) / srcRate
			src[start+j] += 0.6 * math.Exp(-tt/0.008) * math.Sin(2*math.Pi*5000*tt)
		}
	}
	srcPath := filepath.Join(dir, "src.wav")
	dstPath := filepath.Join(dir, "dst.wav")
	if err := wavio.WritePCM16Mono(srcPath, src, srcRate); err != nil {
		t.Fatal(err)
	}
	rs, err := dsp.NewSincResampler(srcRate, dstRate, 18000, 32)
	if err != nil {
		t.Fatal(err)
	}
	converted := rs.Resample(src, dsp.BoundaryZero)
	if err := wavio.WritePCM16Mono(dstPath, append(make([]float64, 11), converted...), dstRate); err != nil {
		t.Fatal(err)
	}

	jsonPath := filepath.Join(dir, "report.json")
	if err := run([]string{"-src", srcPath, "-dst", dstPath, "-fft", "8192", "-format", "json", "-out", jsonPath}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var report analysis.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Source.SampleRate != srcRate || report.Destination.SampleRate != dstRate {
		t.Fatalf("rates: %+v", report)
	}
	if report.GroupDelay.AggregateSpectrumUsed {
		t.Fatal("aggregate spectrum similarity must remain disabled")
	}
	if math.Abs(report.Alignment.LagSamples-11) > 2 {
		t.Fatalf("lag %.3f, want about 11", report.Alignment.LagSamples)
	}

	textPath := filepath.Join(dir, "report.txt")
	if err := run([]string{"-src", srcPath, "-dst", dstPath, "-fft", "8192", "-out", textPath}); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(textPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "重采样时域取证") || !strings.Contains(string(text), "未使用整段平均频谱相似度") {
		t.Fatalf("unexpected text report:\n%s", text)
	}
}

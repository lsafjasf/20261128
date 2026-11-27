package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"yuvdiag"
)

func writeFrame(t *testing.T, path string, width, height int, yValue, cValue uint16, mutate func(y, cb, cr []uint16)) {
	t.Helper()
	cw, ch := (width+1)/2, (height+1)/2
	y := make([]uint16, width*height)
	cb := make([]uint16, cw*ch)
	cr := make([]uint16, cw*ch)
	for i := range y {
		y[i] = yValue
	}
	for i := range cb {
		cb[i], cr[i] = cValue, cValue
	}
	if mutate != nil {
		mutate(y, cb, cr)
	}
	var buf bytes.Buffer
	for _, plane := range [][]uint16{y, cb, cr} {
		for _, v := range plane {
			var b [2]byte
			binary.LittleEndian.PutUint16(b[:], v)
			buf.Write(b[:])
		}
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunJSONReport(t *testing.T) {
	dir := t.TempDir()
	refPath := filepath.Join(dir, "ref.yuv")
	decPath := filepath.Join(dir, "decoded.yuv")
	writeFrame(t, refPath, 4, 4, 512, 512, nil)
	writeFrame(t, decPath, 4, 4, 512, 512, func(y, _, _ []uint16) {
		y[0] = 63
	})
	var stdout, stderr bytes.Buffer
	code, err := run([]string{
		"-input", decPath, "-reference", refPath,
		"-width", "4", "-height", "4", "-json",
	}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("run = %d, %v; stderr=%s", code, err, stderr.String())
	}
	var report yuv.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if report.Summary.TrueOutOfRangePixels != 1 {
		t.Fatalf("true pixels = %d, want 1", report.Summary.TrueOutOfRangePixels)
	}
	if report.Classification.DecodeSamples != 1 ||
		report.Summary.PrimaryOrigin != string(yuv.OriginDecode) {
		t.Fatalf("classification = %+v summary=%+v", report.Classification, report.Summary)
	}
}

func TestRunWritesReportAndFailsOnViolation(t *testing.T) {
	dir := t.TempDir()
	decPath := filepath.Join(dir, "decoded.yuv")
	reportPath := filepath.Join(dir, "report.json")
	writeFrame(t, decPath, 4, 4, 512, 512, func(y, _, _ []uint16) {
		y[0] = 60
	})
	var stdout, stderr bytes.Buffer
	code, err := run([]string{
		"-width", "4", "-height", "4",
		"-report", reportPath, "-fail-on-violation", decPath,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"true_out_of_range_pixels": 1`)) {
		t.Fatalf("report does not contain expected count:\n%s", data)
	}
}

func TestRunRejectsBadGeometry(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-input", "missing.yuv", "-width", "0", "-height", "4"},
		&stdout, &stderr)
	if code != 2 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

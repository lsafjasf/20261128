package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ghostdiag"
	"ghostdiag/internal/synth"
)

func TestRunJSONAndFailOnGhost(t *testing.T) {
	seq := synth.Build()
	dir := t.TempDir()
	original := filepath.Join(dir, "original.y4m")
	denoised := filepath.Join(dir, "denoised.y4m")
	trajectory := filepath.Join(dir, "trajectory.json")
	writeY4M(t, original, seq, seq.Original)
	writeY4M(t, denoised, seq, seq.Denoised)
	data, err := json.Marshal(seq.Trajectory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trajectory, data, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"-original", original, "-denoised", denoised,
		"-trajectory", trajectory, "-output-format", "json",
	}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var report ghostdiag.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout.String())
	}
	if report.FirstGhost == nil || report.FirstGhost.Frame != seq.FirstGhost {
		t.Fatalf("first ghost=%+v", report.FirstGhost)
	}
	if report.Input.Frames != len(seq.Original) {
		t.Fatalf("input frames=%d", report.Input.Frames)
	}
	stdout.Reset()
	stderr.Reset()
	args = append(args, "-fail-on-ghost")
	if code := run(args, &stdout, &stderr); code != 2 {
		t.Fatalf("fail-on-ghost exit=%d, want 2", code)
	}
}

func TestVersionWithoutInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("version output is empty")
	}
}

func writeY4M(t *testing.T, path string, seq synth.Sequence, frames [][]byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := synth.WriteY4M(file, seq, frames); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

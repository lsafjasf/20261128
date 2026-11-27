package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"srtplan/internal/plan"
)

func TestRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "primary.srt")
	secondary := filepath.Join(dir, "secondary.srt")
	report := filepath.Join(dir, "plan.json")
	out := filepath.Join(dir, "merged.srt")

	if err := os.WriteFile(primary, []byte(
		"1\n00:00:00,000 --> 00:00:05,000\nALICE: 你好\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondary, []byte(
		"1\n00:00:01,000 --> 00:00:04,000\nHello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := run([]string{
		"-primary", primary,
		"-secondary", secondary,
		"-report", report,
		"-out", out,
	})
	if err != nil {
		t.Fatal(err)
	}

	merged, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(merged)
	if !strings.Contains(text, "ALICE: 你好") || !strings.Contains(text, "Hello") {
		t.Fatalf("both cues must survive rendering:\n%s", text)
	}

	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var r plan.Report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	if r.Stats.DroppedCues != 0 || r.Stats.ContentCues != 2 {
		t.Fatalf("unexpected stats: %+v", r.Stats)
	}
}

func TestRunRequiresBothInputs(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("missing flags must produce an error")
	}
}

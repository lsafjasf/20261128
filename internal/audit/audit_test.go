package audit

import (
	"path/filepath"
	"testing"

	"frameburn/internal/ppm"
	"frameburn/internal/sequence"
	"frameburn/internal/timeline"
)

func writeFixture(t *testing.T, rate string) (string, *sequence.Manifest) {
	t.Helper()
	plan, err := timeline.Build(timeline.Config{
		Rate: rate, Width: 240, Height: 120, Frames: 24,
		Events: []timeline.Event{
			{Kind: timeline.Pause, Start: 4, Count: 3},
			{Kind: timeline.Black, Start: 10, Count: 2},
			{Kind: timeline.Cut, Start: 14},
			{Kind: timeline.Pause, Start: 18, Count: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifest, err := sequence.Write(plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, manifest
}

func TestAuditCleanSequence(t *testing.T) {
	for _, rate := range []string{"23.976", "29.97DF"} {
		t.Run(rate, func(t *testing.T) {
			dir, _ := writeFixture(t, rate)
			report, err := Audit(dir, DefaultOptions())
			if err != nil {
				t.Fatal(err)
			}
			if !report.OK {
				t.Fatalf("audit failed: %#v", report)
			}
			if len(report.Black.Detected) != 1 || report.Black.Detected[0] != (Run{Start: 10, Count: 2}) {
				t.Fatalf("black runs = %#v", report.Black.Detected)
			}
			if len(report.Cuts.Detected) != 1 || report.Cuts.Detected[0] != 14 {
				t.Fatalf("cuts = %#v", report.Cuts.Detected)
			}
			if len(report.Pauses.Detected) != 2 {
				t.Fatalf("pause runs = %#v", report.Pauses.Detected)
			}
		})
	}
}

func TestAuditDetectsCorruptedFrame(t *testing.T) {
	dir, _ := writeFixture(t, "29.97DF")
	path := filepath.Join(dir, sequence.FrameName(6))
	im, err := ppm.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	im.Fill(ppm.Black)
	if err := im.Save(path); err != nil {
		t.Fatal(err)
	}
	report, err := Audit(dir, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatal("audit accepted a blacked-out, label-less frame")
	}
	if report.Labels.OK && len(report.Black.Unexpected) == 0 {
		t.Fatal("corruption was not reported")
	}
}

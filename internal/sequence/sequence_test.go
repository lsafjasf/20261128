package sequence

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"frameburn/internal/timeline"
)

func TestWriteSequenceAndPauseIsOutsideLabel(t *testing.T) {
	cfg := timeline.Config{
		Rate: "23.976", Width: 240, Height: 120, Frames: 8,
		Events: []timeline.Event{{Kind: timeline.Pause, Start: 3, Count: 2}},
	}
	plan, err := timeline.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifest, err := Write(plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Frames != 8 || manifest.ContentRect.Empty() {
		t.Fatalf("manifest = %#v", manifest)
	}
	f, err := os.Open(filepath.Join(dir, "frames.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	var previous FrameRecord
	for scanner.Scan() {
		var rec FrameRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			t.Fatal(err)
		}
		if rec.Index != int64(count) {
			t.Fatalf("record %d has index %d", count, rec.Index)
		}
		if count == 4 && rec.Timecode != "00:00:00:04" {
			t.Fatalf("label drifted at pause: %#v", rec)
		}
		if count == 4 && rec.ContentFrame != previous.ContentFrame {
			t.Fatal("content should be frozen")
		}
		previous = rec
		count++
	}
	if count != 8 {
		t.Fatalf("got %d records", count)
	}
}

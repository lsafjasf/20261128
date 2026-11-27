package mp4safe

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRealAudioVideoPreservesSampleHashes(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("testdata", "av.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	archived, err := addArchiveExtensions(original)
	if err != nil {
		t.Fatal(err)
	}
	if demoDir := os.Getenv("MP4SAFE_DEMO_DIR"); demoDir != "" {
		if err := os.MkdirAll(demoDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(demoDir, "archived.mp4"), archived, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	inPath := filepath.Join(dir, "archived.mp4")
	outPath := filepath.Join(dir, "stripped.mp4")
	if err := os.WriteFile(inPath, archived, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := InspectFile(inPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(before.Tracks))
	}
	if len(before.Digests.UnknownBoxes) < 2 {
		t.Fatalf("unknown boxes = %d, want at least 2", len(before.Digests.UnknownBoxes))
	}
	result, err := StripPadding(inPath, outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RemovedPadding) < 2 {
		t.Fatalf("removed padding = %d, want at least 2", len(result.RemovedPadding))
	}
	after, err := InspectFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := CompareDigests(before.Digests, after.Digests); err != nil {
		t.Fatal(err)
	}
	for i, track := range after.Digests.Tracks {
		if track.TrackID != before.Digests.Tracks[i].TrackID ||
			track.Handler != before.Digests.Tracks[i].Handler ||
			track.SHA256 != before.Digests.Tracks[i].SHA256 {
			t.Fatalf("track %d digest changed", i)
		}
	}
	var removed uint64
	for _, p := range result.RemovedPadding {
		removed += p.Bytes
	}
	if after.Bytes != before.Bytes-removed {
		t.Fatalf("output bytes = %d, want %d", after.Bytes, before.Bytes-removed)
	}
	if demoDir := os.Getenv("MP4SAFE_DEMO_DIR"); demoDir != "" {
		stripped, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(demoDir, "stripped.mp4"), stripped, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(demoDir, "report.json"), mustJSON(t, result), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func addArchiveExtensions(original []byte) ([]byte, error) {
	f, err := Parse(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		return nil, err
	}
	if len(f.Boxes) == 0 || f.Boxes[0].Type != [4]byte{'f', 't', 'y', 'p'} {
		return nil, fmt.Errorf("ftyp is not first")
	}
	pos := f.Boxes[0].End
	inject := concat(
		box("free", bytes.Repeat([]byte{0xa5}, 24)),
		box("Xtra", []byte("archive-private-extension")),
		uuidBox([16]byte{'0', '1', '2', '3', '4', '5', '6', '7',
			'8', '9', 'a', 'b', 'c', 'd', 'e', 'f'}, []byte("archive-uuid-payload")),
		box("free", bytes.Repeat([]byte{0x5a}, 32)),
	)
	shift := uint64(len(inject))
	if pos > uint64(len(original)) {
		return nil, fmt.Errorf("invalid ftyp end")
	}
	out := make([]byte, 0, len(original)+len(inject))
	out = append(out, original[:pos]...)
	out = append(out, inject...)
	out = append(out, original[pos:]...)
	for _, track := range f.Tracks {
		if track.Chunks == nil {
			continue
		}
		width := uint64(4)
		if track.Chunks.Wide {
			width = 8
		}
		for i, old := range track.Chunks.Entries {
			if old < pos {
				return nil, fmt.Errorf("chunk offset %d precedes insertion", old)
			}
			entry := track.Chunks.EntryStart + uint64(i)*width + shift
			if track.Chunks.Wide {
				binary.BigEndian.PutUint64(out[entry:entry+8], old+shift)
			} else {
				binary.BigEndian.PutUint32(out[entry:entry+4], uint32(old+shift))
			}
		}
	}
	return out, nil
}

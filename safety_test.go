package mp4safe

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func testSpec(wide bool) fixtureSpec {
	video := make([]byte, 60)
	audio := make([]byte, 16)
	for i := range video {
		video[i] = byte(i*7 + 3)
	}
	for i := range audio {
		audio[i] = byte(i*11 + 5)
	}
	return fixtureSpec{
		Wide: wide, VideoSizes: []uint32{10, 20, 30}, AudioSizes: []uint32{7, 9},
		VideoData: video, AudioData: audio, UnknownData: []byte("top-level-private-bytes"),
	}
}

func TestParseBoundariesAndSamples(t *testing.T) {
	for _, wide := range []bool{false, true} {
		t.Run(fmt.Sprintf("wide=%v", wide), func(t *testing.T) {
			data := makeFixture(testSpec(wide))
			f, err := Parse(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if got := len(f.Padding()); got != 3 {
				t.Fatalf("padding boxes = %d, want 3", got)
			}
			for _, b := range f.Padding() {
				if !b.Removable || b.Class != ClassPadding {
					t.Fatalf("padding %s is not removable", b.TypeName())
				}
			}
			if len(f.Tracks) != 2 {
				t.Fatalf("tracks = %d, want 2", len(f.Tracks))
			}
			if got := f.Tracks[0].Handler; got != "vide" {
				t.Fatalf("first handler = %s", got)
			}
			if got := f.Tracks[1].Handler; got != "soun" {
				t.Fatalf("second handler = %s", got)
			}
			digests, err := f.ComputeDigests()
			if err != nil {
				t.Fatal(err)
			}
			if len(digests.Tracks) != 2 || len(digests.MediaPayloads) != 1 {
				t.Fatalf("unexpected digest counts: %+v", digests)
			}
			if len(digests.UnknownBoxes) != 4 {
				t.Fatalf("unknown boxes = %d, want 4", len(digests.UnknownBoxes))
			}
			indexes := 0
			media := 0
			for _, b := range f.allBoxes {
				switch b.Class {
				case ClassIndex:
					indexes++
				case ClassMedia:
					media++
				}
			}
			if indexes != 2 || media != 1 {
				t.Fatalf("boundary classes: index=%d media=%d", indexes, media)
			}
			assertSampleBytes(t, f, 0, testSpec(wide).VideoData)
			assertSampleBytes(t, f, 1, testSpec(wide).AudioData)
		})
	}
}

func assertSampleBytes(t *testing.T, f *File, track int, want []byte) {
	t.Helper()
	var got []byte
	for _, s := range f.Tracks[track].Samples {
		b := make([]byte, s.Size)
		if err := f.readAt(b, s.Offset); err != nil {
			t.Fatal(err)
		}
		got = append(got, b...)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("track %d sample bytes differ", track)
	}
}

func TestStripPaddingPreservesSamplesAndUnknownBytes(t *testing.T) {
	for _, wide := range []bool{false, true} {
		t.Run(fmt.Sprintf("wide=%v", wide), func(t *testing.T) {
			input := makeFixture(testSpec(wide))
			dir := t.TempDir()
			inPath := filepath.Join(dir, "in.mp4")
			outPath := filepath.Join(dir, "out.mp4")
			if err := os.WriteFile(inPath, input, 0o644); err != nil {
				t.Fatal(err)
			}
			before, err := InspectFile(inPath)
			if err != nil {
				t.Fatal(err)
			}
			result, err := StripPadding(inPath, outPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.RemovedPadding) != 3 {
				t.Fatalf("removed padding = %d, want 3", len(result.RemovedPadding))
			}
			if result.UpdatedOffsets != 5 {
				t.Fatalf("updated offsets = %d, want 5", result.UpdatedOffsets)
			}
			after, err := InspectFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := CompareDigests(before.Digests, after.Digests); err != nil {
				t.Fatal(err)
			}
			if len(after.Boxes) != len(before.Boxes)-3 {
				t.Fatalf("top-level boxes = %d, want %d", len(after.Boxes), len(before.Boxes)-3)
			}
			out, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range [][]byte{
				[]byte("top-level-private-bytes"),
				[]byte("uuid-extension-bytes"),
				[]byte("nested-unknown-box"),
			} {
				if !bytes.Contains(out, marker) {
					t.Fatalf("unknown marker %q was not preserved", marker)
				}
			}
			removed := uint64(len(input) - len(out))
			if removed != 32+24+48 {
				t.Fatalf("removed bytes = %d", removed)
			}
			assertRewrittenOffsets(t, inPath, outPath, 56)
		})
	}
}

func assertRewrittenOffsets(t *testing.T, inPath, outPath string, shift uint64) {
	t.Helper()
	before, err := os.Open(inPath)
	if err != nil {
		t.Fatal(err)
	}
	defer before.Close()
	beforeInfo, _ := before.Stat()
	beforeFile, err := Parse(before, beforeInfo.Size())
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	afterInfo, _ := after.Stat()
	afterFile, err := Parse(after, afterInfo.Size())
	if err != nil {
		t.Fatal(err)
	}
	for i := range beforeFile.Tracks {
		for j, old := range beforeFile.Tracks[i].Chunks.Entries {
			got := afterFile.Tracks[i].Chunks.Entries[j]
			if got != old-shift {
				t.Fatalf("track %d chunk %d offset = %d, want %d", i, j, got, old-shift)
			}
		}
	}
}

func TestStripPaddingIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.mp4")
	second := filepath.Join(dir, "second.mp4")
	if err := os.WriteFile(first, makeFixture(testSpec(false)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := StripPadding(first, first); err != nil {
		t.Fatal(err)
	}
	result, err := StripPadding(first, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RemovedPadding) != 0 || result.UpdatedOffsets != 0 {
		t.Fatalf("second pass changed fields: %+v", result)
	}
	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
	if !bytes.Equal(a, b) {
		t.Fatal("already stripped file changed")
	}
	if result.InputSHA256 != result.OutputSHA256 {
		t.Fatal("idempotent rewrite hash mismatch")
	}
}

func TestStripPaddingRejectsFragmentedContainer(t *testing.T) {
	data := append(box("sidx", make([]byte, 12)), makeFixture(testSpec(false))...)
	dir := t.TempDir()
	inPath := filepath.Join(dir, "frag.mp4")
	outPath := filepath.Join(dir, "out.mp4")
	if err := os.WriteFile(inPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := StripPadding(inPath, outPath); err == nil {
		t.Fatal("expected unsupported feature error")
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatal("output should not be committed on rejection")
	}
}

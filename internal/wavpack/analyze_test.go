package wavpack

import (
	"path/filepath"
	"testing"
)

const testFlags uint32 = 0x1 | flagMono | flagHybrid | flagInitial | flagFinal | flagHasChecksum | 10<<23

func TestMissingCorrectionRange(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "audio.wv")
	corr := filepath.Join(dir, "audio.wvc")

	writeFile(t, main, append(
		append(testBlock(0, 1000, 0x11111111, testFlags, nil),
			testBlock(1000, 1000, 0x22222222, testFlags, nil)...),
		testBlock(2000, 1000, 0x33333333, testFlags, nil)...))
	writeFile(t, corr, append(
		testCorrection(0, 1000, 0x11111111, true, true),
		testCorrection(2000, 1000, 0x33333333, true, true)...))

	mainScan, err := ScanFile(main)
	if err != nil {
		t.Fatal(err)
	}
	corrScan, err := ScanFile(corr)
	if err != nil {
		t.Fatal(err)
	}
	analysis := Analyze(mainScan, corrScan)

	if analysis.Complete {
		t.Fatal("analysis unexpectedly complete")
	}
	if len(analysis.MissingRanges) != 1 {
		t.Fatalf("got %d missing ranges, want 1: %+v", len(analysis.MissingRanges), analysis.MissingRanges)
	}
	got := analysis.MissingRanges[0]
	if got.Start != 1000 || got.End != 2000 || got.Samples != 1000 {
		t.Fatalf("missing range = [%d,%d), want [1000,2000)", got.Start, got.End)
	}
}

func TestMatchingCRCMeansCorrectionNotRequired(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "tone.wv")
	corr := filepath.Join(dir, "tone.wvc")

	writeFile(t, main, testBlock(0, 500, 0xdeadbeef, testFlags, nil))
	writeFile(t, corr, testCorrection(0, 500, 0xdeadbeef, false, true))

	mainScan, err := ScanFile(main)
	if err != nil {
		t.Fatal(err)
	}
	corrScan, err := ScanFile(corr)
	if err != nil {
		t.Fatal(err)
	}
	analysis := Analyze(mainScan, corrScan)
	if !analysis.Complete || len(analysis.MissingRanges) != 0 {
		t.Fatalf("unexpected incomplete analysis: %+v", analysis)
	}
	if len(analysis.Dependencies) != 1 || analysis.Dependencies[0].Status != StatusNotRequired {
		t.Fatalf("dependency = %+v, want not_required", analysis.Dependencies)
	}
}

func TestCRCWithoutResidualIsIncomplete(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "lossy.wv")
	corr := filepath.Join(dir, "lossy.wvc")

	writeFile(t, main, testBlock(0, 500, 0x12345678, testFlags, nil))
	writeFile(t, corr, testCorrection(0, 500, 0x87654321, false, true))

	mainScan, err := ScanFile(main)
	if err != nil {
		t.Fatal(err)
	}
	corrScan, err := ScanFile(corr)
	if err != nil {
		t.Fatal(err)
	}
	analysis := Analyze(mainScan, corrScan)
	if analysis.Complete || len(analysis.MissingRanges) != 1 {
		t.Fatalf("analysis = %+v, want one missing range", analysis)
	}
	if analysis.Dependencies[0].Status != StatusIncomplete {
		t.Fatalf("status = %s, want %s", analysis.Dependencies[0].Status, StatusIncomplete)
	}
}

func TestBadCorrectionBlockChecksum(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "keep.wv")
	corr := filepath.Join(dir, "keep.wvc")

	writeFile(t, main, testBlock(0, 500, 0xaaaa0001, testFlags, nil))
	bad := testCorrection(0, 500, 0xbbbb0002, true, true)
	bad[len(bad)-1] ^= 0xff
	writeFile(t, corr, bad)

	mainScan, err := ScanFile(main)
	if err != nil {
		t.Fatal(err)
	}
	corrScan, err := ScanFile(corr)
	if err != nil {
		t.Fatal(err)
	}
	analysis := Analyze(mainScan, corrScan)
	if analysis.Complete || len(analysis.MissingRanges) != 1 {
		t.Fatalf("analysis = %+v, want one missing range", analysis)
	}
	if analysis.Dependencies[0].Status != StatusCorrupt {
		t.Fatalf("status = %s, want %s", analysis.Dependencies[0].Status, StatusCorrupt)
	}
}

func TestBlockIndexHighBits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.wv")
	index := uint64(1)<<32 + 7
	writeFile(t, path, testBlock(index, 100, 0x44556677, testFlags, nil))

	scan, err := ScanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(scan.Blocks))
	}
	if got := scan.Blocks[0].BlockIndex(); got != index {
		t.Fatalf("block index = %d, want %d", got, index)
	}
}

func TestMultichannelCorrectionIsPerStream(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "stereo.wv")
	corr := filepath.Join(dir, "stereo.wvc")
	firstFlags := testFlags &^ flagFinal
	secondFlags := testFlags &^ flagInitial

	writeFile(t, main,
		testBlock(0, 400, 0x11110000, firstFlags, nil),
		testBlock(0, 400, 0x22220000, secondFlags, nil))
	writeFile(t, corr, testCorrection(0, 400, 0x11110000, true, true))

	mainScan, err := ScanFile(main)
	if err != nil {
		t.Fatal(err)
	}
	corrScan, err := ScanFile(corr)
	if err != nil {
		t.Fatal(err)
	}
	analysis := Analyze(mainScan, corrScan)
	if analysis.Complete || len(analysis.MissingRanges) != 1 {
		t.Fatalf("analysis = %+v, want one missing range", analysis)
	}
	r := analysis.MissingRanges[0]
	if r.Start != 0 || r.End != 400 {
		t.Fatalf("range = [%d,%d), want [0,400)", r.Start, r.End)
	}
}

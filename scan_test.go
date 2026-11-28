package mxfindex

import (
	"bytes"
	"testing"
)

func scanFixture(t *testing.T, opts fixtureOptions) *Result {
	t.Helper()
	data := buildFixture(t, opts)
	result, err := Scan(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	return result
}

func TestScanCompleteIndex(t *testing.T) {
	result := scanFixture(t, fixtureOptions{})
	if !result.Success || result.Status != StatusComplete {
		t.Fatalf("status = %s, success = %v, issues = %+v", result.Status, result.Success, result.Issues)
	}
	if len(result.Partitions) != 3 {
		t.Fatalf("partitions = %d, want 3", len(result.Partitions))
	}
	if len(result.IndexSegments) != 1 {
		t.Fatalf("index segments = %d, want 1", len(result.IndexSegments))
	}
	if len(result.Timecodes) != 1 {
		t.Fatalf("timecodes = %d, want 1", len(result.Timecodes))
	}
	if len(result.RebuiltIndex.Tables) != 1 {
		t.Fatalf("tables = %d, want 1", len(result.RebuiltIndex.Tables))
	}
	table := result.RebuiltIndex.Tables[0]
	if !table.Complete || table.StartPosition != 0 || table.EndPosition != 2 {
		t.Fatalf("table = %+v", table)
	}
	if len(table.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(table.Entries))
	}
	if len(result.AddressableRanges) != 1 {
		t.Fatalf("ranges = %d, want 1", len(result.AddressableRanges))
	}
	rng := result.AddressableRanges[0]
	if !rng.Complete || rng.StartEditUnit != 0 || rng.EndEditUnit != 2 {
		t.Fatalf("range = %+v", rng)
	}
	if rng.StartTimecode != "00:00:00:00" || rng.EndTimecode != "00:00:00:01" {
		t.Fatalf("range timecodes = %q..%q", rng.StartTimecode, rng.EndTimecode)
	}
	if table.Entries[0].Offset >= table.Entries[1].Offset {
		t.Fatalf("entry offsets are not ordered: %+v", table.Entries)
	}
}

func TestShortIndexIsIncomplete(t *testing.T) {
	result := scanFixture(t, fixtureOptions{entryCount: 1})
	if result.Success || result.Status != StatusIncomplete {
		t.Fatalf("short index reported as %s/%v", result.Status, result.Success)
	}
	if result.RebuiltIndex.Complete {
		t.Fatal("short index marked complete")
	}
	if len(result.RebuiltIndex.Tables) != 1 || result.RebuiltIndex.Tables[0].Complete {
		t.Fatalf("tables = %+v", result.RebuiltIndex.Tables)
	}
}

func TestDurationLargerThanEntriesIsIncomplete(t *testing.T) {
	result := scanFixture(t, fixtureOptions{indexDuration: 3})
	if result.Success {
		t.Fatal("index with a missing edit unit was reported successful")
	}
}

func TestTrailingUnindexedEssenceIsIncomplete(t *testing.T) {
	result := scanFixture(t, fixtureOptions{entryCount: 1, indexDuration: 1})
	if result.Success {
		t.Fatal("trailing essence beyond the index was reported successful")
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "index.essence_unindexed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing unindexed-essence issue: %+v", result.Issues)
	}
}

func TestClipWrappedEditUnitByteCount(t *testing.T) {
	result := scanFixture(t, fixtureOptions{cbr: true})
	if !result.Success {
		t.Fatalf("CBR fixture failed: %+v", result.Issues)
	}
	table := result.RebuiltIndex.Tables[0]
	if !table.Complete || len(table.Entries) != 2 {
		t.Fatalf("table = %+v", table)
	}
	if table.Entries[0].Offset == table.Entries[1].Offset {
		t.Fatalf("CBR entries mapped to one offset: %+v", table.Entries)
	}
}

func TestMissingIndexIsIncomplete(t *testing.T) {
	result := scanFixture(t, fixtureOptions{noIndex: true})
	if result.Success || result.RebuiltIndex.Complete {
		t.Fatal("file without an index was reported successful")
	}
	if len(result.AddressableRanges) == 0 {
		t.Fatal("essence scan should still expose recovered, non-complete ranges")
	}
	for _, rng := range result.AddressableRanges {
		if rng.Source != "essence_scan" || rng.Complete {
			t.Fatalf("unexpected recovered range: %+v", rng)
		}
	}
}

func TestPartitionBoundaryMismatchIsIncomplete(t *testing.T) {
	result := scanFixture(t, fixtureOptions{badBodyThis: true})
	if result.Success {
		t.Fatal("partition boundary mismatch was reported successful")
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "partition.invalid" || issue.Code == "partition.this_mismatch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("boundary mismatch issue not reported: %+v", result.Issues)
	}
}

func TestDropFrameTimecode(t *testing.T) {
	tests := []struct {
		frame int64
		want  string
	}{
		{0, "00:00:00;00"},
		{1798, "00:00:59;28"},
		{1800, "00:01:00;02"},
		{17982, "00:10:00;00"},
		{19782, "00:11:00;02"},
	}
	for _, tt := range tests {
		if got := FrameToTimecode(tt.frame, 30, true); got != tt.want {
			t.Errorf("FrameToTimecode(%d) = %q, want %q", tt.frame, got, tt.want)
		}
	}
}

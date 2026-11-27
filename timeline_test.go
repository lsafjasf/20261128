package main

import (
	"testing"
	"time"
)

func TestParseSeqDefaultLastDigitRun(t *testing.T) {
	seq, err := ParseSeq("shot12_frame_0042.png", "")
	if err != nil {
		t.Fatal(err)
	}
	if seq != 42 {
		t.Fatalf("got %d, want 42", seq)
	}
}

func TestParseSeqCustomPattern(t *testing.T) {
	seq, err := ParseSeq("frame-7-of-100.exr", `frame-(\d+)`)
	if err != nil {
		t.Fatal(err)
	}
	if seq != 7 {
		t.Fatalf("got %d, want 7", seq)
	}
}

func TestCollectFramesRejectsDuplicates(t *testing.T) {
	_, err := CollectFrames([]string{"a_001.png", "b_001.png"}, "")
	if err == nil {
		t.Fatal("expected duplicate sequence error")
	}
}

func TestFindGaps(t *testing.T) {
	frames, err := CollectFrames([]string{
		"f_0001.png", "f_0002.png", "f_0005.png", "f_0006.png", "f_0010.png",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	gaps := FindGaps(frames)
	if len(gaps) != 2 {
		t.Fatalf("got %d gaps, want 2", len(gaps))
	}
	if gaps[0].AfterSeq != 2 || gaps[0].Len() != 2 || gaps[0].Missing[0] != 3 || gaps[0].Missing[1] != 4 {
		t.Fatalf("unexpected first gap: %+v", gaps[0])
	}
	if gaps[1].AfterSeq != 6 || gaps[1].Len() != 3 {
		t.Fatalf("unexpected second gap: %+v", gaps[1])
	}
}

func TestDurationUsesSequenceNumbersNotFileCount(t *testing.T) {
	frames, err := CollectFrames([]string{"f_0001.png", "f_0002.png", "f_0011.png"}, "")
	if err != nil {
		t.Fatal(err)
	}
	ref := Reference{AnchorSeq: 1, AnchorTime: time.Unix(0, 0).UTC(), FPS: 10}
	got := ref.Duration(frames)
	want := time.Second // (11-1)/10s, not (3-1)/10s
	if got != want {
		t.Fatalf("duration %v, want %v (count-based would be 200ms)", got, want)
	}
}

func TestNewReferenceDerivesFPS(t *testing.T) {
	t0 := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(4 * time.Second)
	ref, err := NewReference(100, t0, 220, t1)
	if err != nil {
		t.Fatal(err)
	}
	if ref.FPS != 30 {
		t.Fatalf("fps %v, want 30", ref.FPS)
	}
	if got := ref.TimeOf(130); !got.Equal(t0.Add(time.Second)) {
		t.Fatalf("TimeOf(130) = %v, want %v", got, t0.Add(time.Second))
	}
}

func TestFillCandidatesFirstGap(t *testing.T) {
	frames, err := CollectFrames([]string{"f_0001.png", "f_0004.png", "f_0005.png"}, "")
	if err != nil {
		t.Fatal(err)
	}
	ref := Reference{AnchorSeq: 1, AnchorTime: time.Unix(0, 0).UTC(), FPS: 25}
	gaps := FindGaps(frames)
	if len(gaps) != 1 {
		t.Fatalf("got %d gaps, want 1", len(gaps))
	}
	cands := FillCandidates(ref, gaps[0])
	if len(cands) != 2 {
		t.Fatalf("got %d candidates, want 2", len(cands))
	}
	if cands[0].Seq != 2 || cands[1].Seq != 3 {
		t.Fatalf("unexpected candidates: %+v", cands)
	}
	if !cands[0].Time.Equal(time.Unix(0, 0).UTC().Add(40 * time.Millisecond)) {
		t.Fatalf("candidate time %v, want 40ms", cands[0].Time)
	}
}

func TestNoGaps(t *testing.T) {
	frames, err := CollectFrames([]string{"f_1.png", "f_2.png", "f_3.png"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if gaps := FindGaps(frames); len(gaps) != 0 {
		t.Fatalf("unexpected gaps: %+v", gaps)
	}
}

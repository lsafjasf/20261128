package project

import (
	"errors"
	"testing"
)

func TestApplyEnforcesContiguousSequence(t *testing.T) {
	p := New("p1")
	p.RegisterMedia(MediaRef{ID: "m1", Path: "a.mp4"})

	err := p.Apply(Operation{Seq: 2, Kind: OpAddClip, ClipID: "c1", MediaID: "m1", Start: 0, End: 1})
	if !errors.Is(err, ErrSeqGap) {
		t.Fatalf("expected ErrSeqGap, got %v", err)
	}

	if err := p.Apply(Operation{Seq: 1, Kind: OpAddClip, ClipID: "c1", MediaID: "m1", Start: 0, End: 1}); err != nil {
		t.Fatalf("apply add: %v", err)
	}
	err = p.Apply(Operation{Seq: 1, Kind: OpRemoveClip, ClipID: "c1"})
	if !errors.Is(err, ErrSeqGap) {
		t.Fatalf("expected duplicate seq rejection, got %v", err)
	}
}

func TestApplyValidatesClipsAndMedia(t *testing.T) {
	p := New("p1")
	p.RegisterMedia(MediaRef{ID: "m1", Path: "a.mp4"})

	if err := p.Apply(Operation{Seq: 1, Kind: OpAddClip, ClipID: "c1", MediaID: "missing", Start: 0, End: 1}); !errors.Is(err, ErrMediaMissing) {
		t.Fatalf("expected ErrMediaMissing, got %v", err)
	}
	if err := p.Apply(Operation{Seq: 1, Kind: OpAddClip, ClipID: "c1", MediaID: "m1", Start: 2, End: 1}); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("expected ErrInvalidRange, got %v", err)
	}
	if err := p.Apply(Operation{Seq: 1, Kind: "weird", ClipID: "c1"}); !errors.Is(err, ErrUnknownOp) {
		t.Fatalf("expected ErrUnknownOp, got %v", err)
	}
	if err := p.Apply(Operation{Seq: 1, Kind: OpAddClip, ClipID: "c1", MediaID: "m1", Start: 0, End: 1}); err != nil {
		t.Fatalf("valid add: %v", err)
	}
	if err := p.Apply(Operation{Seq: 2, Kind: OpMoveClip, ClipID: "c1", Track: 2, Position: 9}); err != nil {
		t.Fatalf("move: %v", err)
	}
	if got := p.Clips["c1"].Track; got != 2 {
		t.Fatalf("track = %d, want 2", got)
	}
	if err := p.Apply(Operation{Seq: 3, Kind: OpTrimClip, ClipID: "c1", Start: 0.2, End: 0.8}); err != nil {
		t.Fatalf("trim: %v", err)
	}
	if err := p.Apply(Operation{Seq: 4, Kind: OpRemoveClip, ClipID: "c1"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, ok := p.Clips["c1"]; ok {
		t.Fatal("clip should have been removed")
	}
}

func TestStateHashStableAndCloneIndependent(t *testing.T) {
	p := New("p1")
	p.RegisterMedia(MediaRef{ID: "m1", Path: "a.mp4"})
	if err := p.Apply(Operation{Seq: 1, Kind: OpAddClip, ClipID: "c1", MediaID: "m1", Start: 0, End: 1}); err != nil {
		t.Fatal(err)
	}
	h1 := p.StateHash()
	cp := p.Clone()
	if cp.StateHash() != h1 {
		t.Fatal("clone hash mismatch")
	}
	if err := cp.Apply(Operation{Seq: 2, Kind: OpRemoveClip, ClipID: "c1"}); err != nil {
		t.Fatal(err)
	}
	if p.StateHash() != h1 {
		t.Fatal("original project mutated through clone")
	}
}

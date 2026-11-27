package captionsession

import (
	"errors"
	"testing"
)

func newTestSession() *Session {
	return newSession(fixedClock{})
}

func TestVersionedUpsertMonotonic(t *testing.T) {
	s := newTestSession()
	op1, err := s.Upsert(Segment{ID: "a", StartPTS: 100, EndPTS: 2000, Text: "hello"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	op2, err := s.Upsert(Segment{ID: "a", StartPTS: 100, EndPTS: 2000, Text: "hello world"})
	if err != nil {
		t.Fatalf("upsert revise: %v", err)
	}
	if op2.Version != op1.Version+1 {
		t.Fatalf("versions not monotonic: %d -> %d", op1.Version, op2.Version)
	}
	st := s.State()
	if got := st.Segments(); len(got) != 1 || got[0].Text != "hello world" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if s.Version() != 2 {
		t.Fatalf("session version = %d, want 2", s.Version())
	}
}

func TestFrozenSegmentRejectsBackfill(t *testing.T) {
	s := newTestSession()
	if _, err := s.Upsert(Segment{ID: "a", StartPTS: 100, EndPTS: 2000, Text: "v1"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AdvanceWatermark(2000); err != nil {
		t.Fatal(err)
	}

	// ASR revision of an already-displayed segment must be refused.
	if _, err := s.Upsert(Segment{ID: "a", StartPTS: 100, EndPTS: 2000, Text: "v2"}); !errors.Is(err, ErrSegmentFrozen) {
		t.Fatalf("want ErrSegmentFrozen, got %v", err)
	}
	// Operator replacement must be refused too: no retroactive edit.
	if _, err := s.Replace("a", "corrected"); !errors.Is(err, ErrSegmentFrozen) {
		t.Fatalf("replace want ErrSegmentFrozen, got %v", err)
	}
	// A brand-new segment landing entirely in the past is also refused.
	if _, err := s.Upsert(Segment{ID: "late", StartPTS: 0, EndPTS: 1500, Text: "x"}); !errors.Is(err, ErrSegmentFrozen) {
		t.Fatalf("late upsert want ErrSegmentFrozen, got %v", err)
	}
	st := s.State()
	got, _ := st.Get("a")
	if got.Text != "v1" {
		t.Fatalf("displayed content mutated: %q", got.Text)
	}
}

func TestLocalReplaceDoesNotReflow(t *testing.T) {
	s := newTestSession()
	segs := []Segment{
		{ID: "a", StartPTS: 0, EndPTS: 1000, Text: "one"},
		{ID: "b", StartPTS: 1000, EndPTS: 2000, Text: "to"},
		{ID: "c", StartPTS: 2000, EndPTS: 3000, Text: "three"},
	}
	for _, seg := range segs {
		if _, err := s.Upsert(seg); err != nil {
			t.Fatal(err)
		}
	}
	op, err := s.Replace("b", "two")
	if err != nil {
		t.Fatal(err)
	}
	if op.Kind != OpReplace || op.SegmentID != "b" || op.OldText != "to" || op.NewText != "two" {
		t.Fatalf("bad replace op: %+v", op)
	}
	got := s.State().Segments()
	wantOrder := []string{"a", "b", "c"}
	for i, id := range wantOrder {
		if got[i].ID != id {
			t.Fatalf("order changed after replace: %+v", got)
		}
	}
	if got[1].Text != "two" {
		t.Fatalf("replace not applied: %q", got[1].Text)
	}
}

func TestReplaceMissingSegment(t *testing.T) {
	s := newTestSession()
	if _, err := s.Replace("ghost", "x"); !errors.Is(err, ErrSegmentNotFound) {
		t.Fatalf("want ErrSegmentNotFound, got %v", err)
	}
}

func TestWatermarkMonotonic(t *testing.T) {
	s := newTestSession()
	if _, _, err := s.AdvanceWatermark(500); err != nil {
		t.Fatal(err)
	}
	if _, moved, err := s.AdvanceWatermark(400); err != nil || moved {
		t.Fatalf("backward advance must be no-op, moved=%v err=%v", moved, err)
	}
	if s.Watermark() != 500 {
		t.Fatalf("watermark = %d, want 500", s.Watermark())
	}
}

func TestDeterministicReplayParity(t *testing.T) {
	s := newTestSession()
	_, _ = s.Upsert(Segment{ID: "a", StartPTS: 0, EndPTS: 1000, Text: "one"})
	_, _ = s.Upsert(Segment{ID: "b", StartPTS: 1000, EndPTS: 2000, Text: "to"})
	_, _, _ = s.AdvanceWatermark(1000)
	_, _ = s.Replace("b", "two")
	_, _, _ = s.AdvanceWatermark(2000)

	// A reconnecting viewer with only the initial state replays every op.
	_, ops, err := s.Replay(InitialVersion)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := Apply(nil, ops)
	if err != nil {
		t.Fatal(err)
	}
	live := s.State()
	if !statesEqual(replayed, live) {
		t.Fatalf("replay diverged from live:\nreplay=%+v\nlive=%+v",
			replayed.Segments(), live.Segments())
	}
}

func statesEqual(a, b *State) bool {
	if a.Version != b.Version || a.Watermark != b.Watermark {
		return false
	}
	sa, sb := a.Segments(), b.Segments()
	if len(sa) != len(sb) {
		return false
	}
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

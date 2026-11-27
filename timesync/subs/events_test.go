package subs

import (
	"testing"

	"timesync/remap"
)

func TestRemapPreservesTextAndMapsLocally(t *testing.T) {
	curve, err := remap.NewCurve([]remap.Segment{
		{S0: 0, S1: 2, RateStart: 1, RateEnd: 1},
		{S0: 2, S1: 4, RateStart: 0.5, RateEnd: 0.5},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := []Event{
		{ID: "w1", Text: "hello", Start: 0.5, End: 1.5},
		{ID: "w2", Text: "world", Start: 2.5, End: 3.5},
	}
	mapped, err := Remap(events, curve)
	if err != nil {
		t.Fatal(err)
	}
	if !TextPreserved(events, mapped) || !OrderingIntact(mapped) {
		t.Fatal("text/order invariants broken")
	}
	// Second event lives in the 0.5x segment: its output span doubles.
	if mapped[1].StartOut != 3 || mapped[1].EndOut != 5 {
		t.Fatalf("event 2 mapped to %g-%g, want 3-5", mapped[1].StartOut, mapped[1].EndOut)
	}
	// The per-event drift differs across segments, proving no global shift.
	if mapped[0].StartOut-mapped[0].Start == mapped[1].StartOut-mapped[1].Start {
		t.Fatal("events share one global offset instead of local mapping")
	}
}

func TestRemapRejectsOutOfRange(t *testing.T) {
	curve, _ := remap.NewCurve([]remap.Segment{
		{S0: 0, S1: 1, RateStart: 1, RateEnd: 1},
	})
	if _, err := Remap([]Event{{Text: "x", Start: 0.5, End: 1.5}}, curve); err == nil {
		t.Fatal("expected out-of-range error")
	}
	if _, err := Remap([]Event{{Text: "x", Start: 0.8, End: 0.2}}, curve); err == nil {
		t.Fatal("expected inverted-interval error")
	}
}

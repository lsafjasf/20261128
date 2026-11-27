// Package subs remaps word-level subtitle events with the same curve used
// for audio. Text payloads are never modified; only timestamps move, and
// each event is mapped through the segment that contains it rather than
// shifted by one global offset.
package subs

import (
	"fmt"
	"timesync/remap"
)

// Event is one word-level subtitle cue.
type Event struct {
	ID    string
	Text  string
	Start float64 // source time, seconds
	End   float64 // source time, seconds
}

// MappedEvent is an event expressed on the remapped output timeline.
type MappedEvent struct {
	Event
	StartOut float64
	EndOut   float64
}

// Remap maps every event independently through the curve. Ordering and
// text are preserved, and start/end are checked against the source range.
func Remap(events []Event, c *remap.Curve) ([]MappedEvent, error) {
	sourceDur := c.SourceDuration()
	out := make([]MappedEvent, 0, len(events))
	for i, ev := range events {
		if ev.Start < 0 || ev.End < ev.Start {
			return nil, fmt.Errorf("subs: event %d has invalid timing [%g,%g]", i, ev.Start, ev.End)
		}
		if ev.End > sourceDur+1e-9 {
			return nil, fmt.Errorf("subs: event %d ends at %g beyond source duration %g", i, ev.End, sourceDur)
		}
		out = append(out, MappedEvent{
			Event:    ev,
			StartOut: c.Map(ev.Start),
			EndOut:   c.Map(ev.End),
		})
	}
	return out, nil
}

// OrderingIntact reports whether mapped events stay non-overlapping in the
// same order as the input. A monotonic remap (rate > 0) must preserve it.
func OrderingIntact(mapped []MappedEvent) bool {
	for i := 1; i < len(mapped); i++ {
		if mapped[i].StartOut < mapped[i-1].StartOut-1e-12 {
			return false
		}
		if mapped[i].EndOut < mapped[i-1].EndOut-1e-12 {
			return false
		}
	}
	return true
}

// TextPreserved reports that no text payload changed during remapping.
func TextPreserved(original []Event, mapped []MappedEvent) bool {
	if len(original) != len(mapped) {
		return false
	}
	for i := range original {
		if original[i].Text != mapped[i].Text {
			return false
		}
	}
	return true
}

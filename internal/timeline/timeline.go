// Package timeline builds a deterministic per-output-frame plan.
package timeline

import (
	"errors"
	"fmt"
	"sort"

	"frameburn/internal/burnin"
	"frameburn/internal/timecode"
)

type EventKind string

const (
	Cut   EventKind = "cut"
	Black EventKind = "black"
	Pause EventKind = "pause"
)

var (
	ErrInvalidConfig = errors.New("invalid timeline")
	ErrEventOverlap  = errors.New("timeline events overlap")
)

// Event uses zero-based output frame indexes. A black or pause event covers
// Count frames starting at Start.
type Event struct {
	Kind  EventKind `json:"kind"`
	Start int       `json:"start"`
	Count int       `json:"count,omitempty"`
}

func (e Event) end() int {
	if e.Count > 0 {
		return e.Start + e.Count
	}
	return e.Start + 1
}

type Config struct {
	Rate   string  `json:"rate"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Frames int     `json:"frames"`
	Events []Event `json:"events"`
}

func DefaultConfig(rate string) Config {
	return DefaultConfigSized(rate, 480, 270, 150)
}

func DefaultConfigSized(rate string, width, height, frames int) Config {
	cfg := Config{
		Rate:   rate,
		Width:  width,
		Height: height,
		Frames: frames,
	}
	cfg.Events = demoEvents(cfg.Frames)
	return cfg
}

func demoEvents(frames int) []Event {
	if frames < 24 {
		if frames >= 6 {
			return []Event{{Kind: Cut, Start: frames / 2}}
		}
		return nil
	}
	pauseOne := frames * 30 / 100
	pauseOneCount := max(2, frames*16/100)
	blackStart := pauseOne + pauseOneCount + max(2, frames*10/100)
	blackCount := min(2, frames-blackStart)
	cut := blackStart + blackCount + max(2, frames*10/100)
	pauseTwo := cut + max(2, frames*10/100)
	events := []Event{
		{Kind: Pause, Start: pauseOne, Count: pauseOneCount},
		{Kind: Black, Start: blackStart, Count: blackCount},
		{Kind: Cut, Start: cut},
	}
	if remaining := frames - pauseTwo; remaining >= 2 {
		events = append(events, Event{Kind: Pause, Start: pauseTwo, Count: min(remaining, max(2, frames*8/100))})
	}
	return events
}

type Frame struct {
	Index        int64
	Stamp        burnin.Stamp
	ContentFrame int64
	Clip         int
	Black        bool
	Paused       bool
}

type Plan struct {
	Config   Config
	Rate     timecode.Rate
	Events   []Event
	Frames   []Frame
	CutSet   map[int]bool
	BlackSet map[int]bool
	PauseSet map[int]bool
}

func Build(cfg Config) (*Plan, error) {
	cfg = normalize(cfg)
	rate, err := timecode.ParseRate(cfg.Rate)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if cfg.Width < 64 || cfg.Height < 48 || cfg.Frames <= 0 {
		return nil, fmt.Errorf("%w: dimensions or frame count too small", ErrInvalidConfig)
	}
	events := append([]Event(nil), cfg.Events...)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Start != events[j].Start {
			return events[i].Start < events[j].Start
		}
		return events[i].Kind < events[j].Kind
	})
	events = mergeAdjacent(events)
	if err := validateEvents(events, cfg.Frames); err != nil {
		return nil, err
	}
	plan := &Plan{
		Config:   cfg,
		Rate:     rate,
		Events:   events,
		Frames:   make([]Frame, cfg.Frames),
		CutSet:   make(map[int]bool),
		BlackSet: make(map[int]bool),
		PauseSet: make(map[int]bool),
	}
	for _, event := range events {
		for frame := event.Start; frame < event.end(); frame++ {
			switch event.Kind {
			case Cut:
				plan.CutSet[frame] = true
			case Black:
				plan.BlackSet[frame] = true
			case Pause:
				plan.PauseSet[frame] = true
			}
		}
	}
	nextContent := int64(0)
	clip := 0
	for i := 0; i < cfg.Frames; i++ {
		if plan.CutSet[i] {
			clip++
		}
		contentFrame := nextContent
		paused := plan.PauseSet[i]
		if paused {
			contentFrame = max(int64(0), nextContent-1)
		} else {
			nextContent++
		}
		stamp, err := burnin.NewStamp(rate, int64(i))
		if err != nil {
			return nil, err
		}
		plan.Frames[i] = Frame{
			Index:        int64(i),
			Stamp:        stamp,
			ContentFrame: contentFrame,
			Clip:         clip,
			Black:        plan.BlackSet[i],
			Paused:       paused,
		}
	}
	return plan, nil
}

func normalize(cfg Config) Config {
	if cfg.Width == 0 {
		cfg.Width = 480
	}
	if cfg.Height == 0 {
		cfg.Height = 270
	}
	return cfg
}

func validateEvents(events []Event, frames int) error {
	var intervals []Event
	seenCuts := make(map[int]bool)
	for i, event := range events {
		switch event.Kind {
		case Cut:
			if event.Count != 0 {
				return fmt.Errorf("%w: cut event must not have a count", ErrInvalidConfig)
			}
			if event.Start <= 0 || event.Start >= frames {
				return fmt.Errorf("%w: cut at %d outside (0,%d)", ErrInvalidConfig, event.Start, frames)
			}
			if seenCuts[event.Start] {
				return fmt.Errorf("%w: duplicate cut at %d", ErrInvalidConfig, event.Start)
			}
			seenCuts[event.Start] = true
		case Black, Pause:
			if event.Count <= 0 || event.Start < 0 || event.end() > frames {
				return fmt.Errorf("%w: %s event %d does not fit in %d frames", ErrInvalidConfig, event.Kind, i, frames)
			}
			if event.Kind == Pause && event.Start == 0 {
				return fmt.Errorf("%w: pause must have a previous content frame", ErrInvalidConfig)
			}
			intervals = append(intervals, event)
		default:
			return fmt.Errorf("%w: unknown event kind %q", ErrInvalidConfig, event.Kind)
		}
	}
	for i := 1; i < len(intervals); i++ {
		prev, cur := intervals[i-1], intervals[i]
		if cur.Start < prev.end() {
			return fmt.Errorf("%w: %s at %d and %s at %d", ErrEventOverlap, prev.Kind, prev.Start, cur.Kind, cur.Start)
		}
	}
	for cut := range seenCuts {
		for _, interval := range intervals {
			if cut >= interval.Start && cut <= interval.end() {
				return fmt.Errorf("%w: cut at %d is inside %s event at %d", ErrEventOverlap, cut, interval.Kind, interval.Start)
			}
		}
	}
	for i := 1; i < len(intervals); i++ {
		prev, cur := intervals[i-1], intervals[i]
		if prev.Kind == Black && cur.Kind == Pause && prev.end() == cur.Start {
			return fmt.Errorf("%w: pause at %d starts immediately after black", ErrEventOverlap, cur.Start)
		}
	}
	return nil
}

func mergeAdjacent(events []Event) []Event {
	merged := make([]Event, 0, len(events))
	for _, event := range events {
		if event.Kind != Cut && len(merged) > 0 {
			prev := &merged[len(merged)-1]
			if prev.Kind == event.Kind && prev.end() == event.Start {
				prev.Count += event.Count
				continue
			}
		}
		merged = append(merged, event)
	}
	return merged
}

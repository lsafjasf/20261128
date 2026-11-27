package subburn

import (
	"fmt"
	"time"
)

// RenderClock describes how an encoder decides which subtitle is active for a
// frame. TrueTime is when the frame is actually displayed. QueryTime is the
// timestamp handed to the subtitle selector. A correct renderer makes them
// equal; the defect fields exist so an audit can reproduce and isolate errors.
type RenderClock struct {
	Rate Rate

	// FrameStep replaces exact frame timestamps with frame*FrameStep. Integer
	// millisecond stepping is a common source of cumulative drift.
	FrameStep time.Duration

	// ReportedRate replaces exact timestamps with timestamps computed at a
	// different rate, for example treating 30000/1001 as 30.
	ReportedRate Rate

	// Preroll is encoder lead-in that must not move the subtitle display clock.
	// IncludePreroll reproduces the bug where the selector uses encoded PTS
	// instead of display PTS.
	Preroll        time.Duration
	IncludePreroll bool

	// Segment is an encoded chunk length. SegmentPreroll is added again for
	// every chunk, producing sawtooth jumps rather than a linear slope.
	Segment        time.Duration
	SegmentPreroll time.Duration

	// SubtitleOrigin is a nonzero subtitle/import origin that was not removed.
	SubtitleOrigin time.Duration

	// DisplayStart is the container's first presentation timestamp. It must be
	// normalized before subtitle lookup; IncludeDisplayStart models the bug.
	DisplayStart        time.Duration
	IncludeDisplayStart bool
}

func ExactClock(rate Rate) RenderClock {
	return RenderClock{Rate: rate}
}

func (c RenderClock) Validate() error {
	if !c.Rate.Valid() {
		return fmt.Errorf("invalid render rate: %s", c.Rate)
	}
	if c.FrameStep < 0 {
		return fmt.Errorf("negative frame step: %s", c.FrameStep)
	}
	if c.ReportedRate.Num != 0 || c.ReportedRate.Den != 0 {
		if !c.ReportedRate.Valid() {
			return fmt.Errorf("invalid reported rate: %s", c.ReportedRate)
		}
	}
	if c.Preroll < 0 || c.SegmentPreroll < 0 || c.DisplayStart < 0 {
		return fmt.Errorf("clock offsets must not be negative")
	}
	if c.SegmentPreroll > 0 && c.Segment <= 0 {
		return fmt.Errorf("segment preroll requires a positive segment duration")
	}
	return nil
}

// TrueTime is the frame's presentation time relative to the first displayed
// frame. It is always derived from the exact rational rate.
func (c RenderClock) TrueTime(frame int64) time.Duration {
	return c.Rate.FramePTS(frame)
}

// QueryTime is the timestamp the renderer uses to choose a subtitle. For a
// correct clock it equals TrueTime.
func (c RenderClock) QueryTime(frame int64) time.Duration {
	base := c.Rate.FramePTS(frame)
	query := base
	if c.FrameStep > 0 {
		query = time.Duration(frame) * c.FrameStep
	}
	if c.ReportedRate.Valid() {
		query = c.ReportedRate.FramePTS(frame)
	}
	if c.IncludePreroll {
		query += c.Preroll
	}
	if c.SegmentPreroll > 0 && c.Segment > 0 {
		segments := base / c.Segment
		query += time.Duration(segments) * c.SegmentPreroll
	}
	query += c.SubtitleOrigin
	if c.IncludeDisplayStart {
		query += c.DisplayStart
	}
	return query
}

// OnsetFrame finds the first frame whose subtitle query time is at or after
// start. maxDisplayTime bounds the search in true display time.
func (c RenderClock) OnsetFrame(start, maxDisplayTime time.Duration) (int64, bool) {
	if start <= 0 {
		return 0, true
	}
	if err := c.Validate(); err != nil {
		return 0, false
	}
	maxFrame := c.Rate.FrameAt(maxDisplayTime) + 2
	step := c.Rate.FramePTS(1)
	if c.FrameStep > 0 {
		step = c.FrameStep
	}
	if c.ReportedRate.Valid() {
		step = c.ReportedRate.FramePTS(1)
	}
	if step <= 0 {
		return 0, false
	}
	needed := int64(start/step) + int64(absDuration(c.SubtitleOrigin)/step) + 2
	if needed > maxFrame {
		maxFrame = needed
	}
	if c.QueryTime(0) >= start {
		return 0, true
	}
	lo, hi := int64(0), int64(1)
	for hi <= maxFrame && c.QueryTime(hi) < start {
		lo = hi
		hi *= 2
	}
	if hi > maxFrame && c.QueryTime(maxFrame) < start {
		return 0, false
	}
	if hi > maxFrame {
		hi = maxFrame
	}
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		if c.QueryTime(mid) >= start {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi, true
}

// CueSpan returns frames visible when the selector scans QueryTime over
// [cue.Start, cue.End). The end frame is exclusive.
func (c RenderClock) CueSpan(cue Cue, maxDisplayTime time.Duration) (FrameSpan, bool) {
	if err := cue.Validate(); err != nil {
		return FrameSpan{}, false
	}
	first, ok := c.OnsetFrame(cue.Start, maxDisplayTime)
	if !ok {
		return FrameSpan{}, false
	}
	end, ok := c.OnsetFrame(cue.End, maxDisplayTime)
	if !ok {
		return FrameSpan{}, false
	}
	if end <= first {
		return FrameSpan{}, false
	}
	return FrameSpan{First: first, End: end}, true
}

type FrameSpan struct {
	First int64
	End   int64
}

func (s FrameSpan) Count() int64 {
	if s.End <= s.First {
		return 0
	}
	return s.End - s.First
}

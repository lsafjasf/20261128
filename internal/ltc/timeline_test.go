package ltc

import (
	"testing"
	"time"
)

func mjdForDate(t *testing.T, date string) int {
	t.Helper()
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(1858, time.November, 17, 0, 0, 0, 0, time.UTC)
	return int(d.Sub(base).Hours() / 24)
}

func datedFrame(t *testing.T, tc Timecode, date string) Frame {
	t.Helper()
	groups := [8]uint8{}
	setDateMJD(&groups, mjdForDate(t, date))
	return testFrame(t, tc, 30, false, groups, false, false, true)
}

func undatedFrame(t *testing.T, tc Timecode) Frame {
	t.Helper()
	return testFrame(t, tc, 30, false, [8]uint8{}, false, false, false)
}

func hasEvent(t Timeline, typ string) bool {
	for _, e := range t.Events {
		if e.Type == typ {
			return true
		}
	}
	return false
}

func TestCrossDayWithMJDDate(t *testing.T) {
	frames := []Frame{
		datedFrame(t, Timecode{23, 59, 59, 29}, "2026-11-27"),
		datedFrame(t, Timecode{0, 0, 0, 0}, "2026-11-28"),
	}
	tl := Analyze("camera-a", frames, 30)

	if len(tl.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(tl.Segments))
	}
	s := tl.Segments[0]
	if !s.CrossesMidnight || s.StartDate != "2026-11-27" || s.EndDate != "2026-11-28" {
		t.Fatalf("segment = %+v", s)
	}
	if s.Ambiguous {
		t.Fatalf("dated midnight segment should be deterministic: %+v", s)
	}
	if !hasEvent(tl, EventMidnightRollover) || !hasEvent(tl, EventDateRollover) {
		t.Fatalf("events = %+v", tl.Events)
	}
	if tl.Frames[0].RelativeDay != 0 || tl.Frames[1].RelativeDay != 1 {
		t.Fatalf("relative days = %d,%d", tl.Frames[0].RelativeDay, tl.Frames[1].RelativeDay)
	}
}

func TestCrossDayWithoutDateStaysAmbiguous(t *testing.T) {
	frames := []Frame{
		undatedFrame(t, Timecode{23, 59, 59, 29}),
		undatedFrame(t, Timecode{0, 0, 0, 0}),
	}
	tl := Analyze("camera-b", frames, 30)

	if len(tl.Segments) != 1 || tl.Segments[0].DateStatus != DateAbsent {
		t.Fatalf("segments = %+v", tl.Segments)
	}
	if !tl.Segments[0].Ambiguous || tl.Segments[0].RelativeDayEnd != 1 {
		t.Fatalf("segment = %+v", tl.Segments[0])
	}
	if len(tl.Ambiguities) == 0 {
		t.Fatal("missing date ambiguity was not reported")
	}
	if hasEvent(tl, EventDateRollover) || hasEvent(tl, EventDateNotAdvanced) {
		t.Fatal("absent date bits must not be treated as a date rollover")
	}
}

func TestStaticDateAcrossMidnightIsContradictory(t *testing.T) {
	frames := []Frame{
		datedFrame(t, Timecode{23, 59, 59, 29}, "2026-11-27"),
		datedFrame(t, Timecode{0, 0, 0, 0}, "2026-11-27"),
	}
	tl := Analyze("loop", frames, 30)

	if !hasEvent(tl, EventMidnightRollover) || !hasEvent(tl, EventDateNotAdvanced) {
		t.Fatalf("events = %+v", tl.Events)
	}
	if len(tl.Segments) != 2 || !tl.Segments[1].Ambiguous {
		t.Fatalf("segments = %+v", tl.Segments)
	}
}

func TestDuplicateAndRollback(t *testing.T) {
	frames := []Frame{
		undatedFrame(t, Timecode{0, 0, 10, 0}),
		undatedFrame(t, Timecode{0, 0, 10, 1}),
		undatedFrame(t, Timecode{0, 0, 10, 1}),
		undatedFrame(t, Timecode{0, 0, 9, 29}),
	}
	tl := Analyze("edits", frames, 30)

	if !hasEvent(tl, EventDuplicate) || !hasEvent(tl, EventRollback) {
		t.Fatalf("events = %+v", tl.Events)
	}
	if len(tl.Segments) != 3 {
		t.Fatalf("segments = %d, want 3", len(tl.Segments))
	}
	if !tl.Frames[2].ChronologyAmbiguous || !tl.Frames[3].ChronologyAmbiguous {
		t.Fatalf("chronology flags = %+v", tl.Frames)
	}
}

func TestDateJump(t *testing.T) {
	frames := []Frame{
		datedFrame(t, Timecode{12, 0, 0, 0}, "2026-11-27"),
		datedFrame(t, Timecode{12, 0, 0, 1}, "2026-11-29"),
	}
	tl := Analyze("jump", frames, 30)

	if !hasEvent(tl, EventDateJump) {
		t.Fatalf("events = %+v", tl.Events)
	}
	if tl.Frames[1].RelativeDay != 2 {
		t.Fatalf("relative day = %d, want 2", tl.Frames[1].RelativeDay)
	}
}

func TestDropFrameMinuteBoundaryIsSequential(t *testing.T) {
	mk := func(tc Timecode, drop bool) Frame {
		return testFrame(t, tc, 30, drop, [8]uint8{}, false, false, false)
	}
	frames := []Frame{
		mk(Timecode{0, 0, 59, 28}, true),
		mk(Timecode{0, 0, 59, 29}, true),
		mk(Timecode{0, 1, 0, 2}, true),
	}
	tl := Analyze("df", frames, 30)
	if len(tl.Events) != 0 {
		t.Fatalf("drop-frame minute boundary produced events: %+v", tl.Events)
	}
	if tl.Frames[2].Chronology != "sequential" {
		t.Fatalf("chronology = %s", tl.Frames[2].Chronology)
	}
}

func TestCrossSourceOrdering(t *testing.T) {
	a := Analyze("a", []Frame{
		datedFrame(t, Timecode{23, 59, 59, 29}, "2026-11-27"),
		datedFrame(t, Timecode{0, 0, 0, 0}, "2026-11-28"),
	}, 30)
	b := Analyze("b", []Frame{
		undatedFrame(t, Timecode{23, 59, 59, 29}),
		undatedFrame(t, Timecode{0, 0, 0, 0}),
	}, 30)

	merge := CrossSource([]Timeline{a, b})
	if merge.Ordering != "ambiguous" {
		t.Fatalf("merge = %+v", merge)
	}

	merge = CrossSource([]Timeline{a})
	if merge.Ordering != "determined" {
		t.Fatalf("single dated merge = %+v", merge)
	}
}

func TestLateDateAnchorKeepsInferredDay(t *testing.T) {
	frames := []Frame{
		undatedFrame(t, Timecode{Hours: 23, Minutes: 59, Seconds: 59, Frames: 29}),
		undatedFrame(t, Timecode{Hours: 0, Minutes: 0, Seconds: 0, Frames: 0}),
		datedFrame(t, Timecode{Hours: 0, Minutes: 0, Seconds: 0, Frames: 1}, "2026-11-28"),
	}
	tl := Analyze("late-anchor", frames, 30)
	if tl.Frames[2].RelativeDay != 1 {
		t.Fatalf("late date anchor moved relative day to %d, want 1", tl.Frames[2].RelativeDay)
	}
	if len(tl.Segments) != 2 || tl.Segments[0].DateStatus != DateAbsent {
		t.Fatalf("segments = %+v", tl.Segments)
	}
}

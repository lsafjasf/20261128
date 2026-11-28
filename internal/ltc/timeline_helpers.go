package ltc

import (
	"fmt"
	"time"
)

func calendarKey(info DateInfo) string {
	switch info.Status {
	case DateDetermined:
		return "D:" + info.Date
	case DateCenturyAmbiguous:
		return "Y:" + info.Raw
	default:
		return ""
	}
}

// compareDates returns the whole-day delta from a to b. The second result is
// false when the two encodings cannot establish a calendar delta.
func compareDates(a, b DateInfo) (int, bool) {
	if a.Status == DateDetermined && b.Status == DateDetermined {
		da, errA := time.Parse("2006-01-02", a.Date)
		db, errB := time.Parse("2006-01-02", b.Date)
		if errA == nil && errB == nil {
			return int(db.Sub(da).Hours() / 24), true
		}
	}
	if a.Status == DateCenturyAmbiguous && b.Status == DateCenturyAmbiguous {
		da, errA := parseDDMMYY(a.Raw)
		db, errB := parseDDMMYY(b.Raw)
		if errA == nil && errB == nil {
			delta := int(db.Sub(da).Hours() / 24)
			if delta > -180 && delta < 180 {
				return delta, true
			}
		}
	}
	return 0, false
}

func parseDDMMYY(raw string) (time.Time, error) {
	return time.Parse("02-01-06", raw)
}

func finalizeSegment(t *Timeline, start, end, index int) {
	if start < 0 || end < start || end >= len(t.Frames) {
		return
	}
	first := &t.Frames[start]
	last := &t.Frames[end]
	status := first.Date.Status
	chronAmbiguous := false
	for i := start; i <= end; i++ {
		if t.Frames[i].Date.Status != status {
			status = DateMixed
		}
		if t.Frames[i].ChronologyAmbiguous {
			chronAmbiguous = true
		}
	}

	s := Segment{
		Index:            index,
		StartFrameIndex:  start,
		EndFrameIndex:    end,
		StartTimecode:    first.TimeAddress,
		EndTimecode:      last.TimeAddress,
		DateStatus:       status,
		RelativeDayStart: first.RelativeDay,
		RelativeDayEnd:   last.RelativeDay,
		CrossesMidnight:  first.RelativeDay != last.RelativeDay,
		FrameCount:       end - start + 1,
		Chronology:       "continuous",
	}
	if status == DateDetermined {
		s.StartDate = first.Date.Date
		s.EndDate = last.Date.Date
		s.StartUTC = frameUTC(*first, t.NominalFPS)
		s.EndUTC = frameUTC(*last, t.NominalFPS)
	}
	if chronAmbiguous {
		s.Chronology = "discontinuous"
		s.Ambiguous = true
		s.AmbiguityReasons = append(s.AmbiguityReasons,
			"segment contains or begins after a time-address/calendar discontinuity")
	}
	switch status {
	case DateAbsent:
		s.Ambiguous = true
		s.AmbiguityReasons = append(s.AmbiguityReasons,
			"no calendar date is declared by LTC user bits; only a relative day can be inferred")
	case DateCenturyAmbiguous:
		s.Ambiguous = true
		s.AmbiguityReasons = append(s.AmbiguityReasons,
			"date is DDMMYY with no century; absolute calendar placement is unresolved")
	case DateInvalid:
		s.Ambiguous = true
		s.AmbiguityReasons = append(s.AmbiguityReasons,
			"ST 309 date bits are invalid")
	case DateMixed:
		s.Ambiguous = true
		s.AmbiguityReasons = append(s.AmbiguityReasons,
			"calendar evidence changes within the segment")
	}
	t.Segments = append(t.Segments, s)
}

func frameUTC(f FrameObservation, nominalFPS int) string {
	if f.Date.Status != DateDetermined {
		return ""
	}
	d, err := time.Parse("2006-01-02", f.Date.Date)
	if err != nil {
		return ""
	}
	tc := f.Timecode
	instant := time.Date(d.Year(), d.Month(), d.Day(), tc.Hours, tc.Minutes, tc.Seconds, 0, time.UTC)
	if nominalFPS > 0 {
		instant = instant.Add(time.Duration(float64(tc.Frames) * float64(time.Second) / float64(nominalFPS)))
	}
	return instant.Format(time.RFC3339Nano)
}

func buildAmbiguities(t Timeline) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	absent, century, invalid := 0, 0, 0
	for _, f := range t.Frames {
		switch f.Date.Status {
		case DateAbsent:
			absent++
		case DateCenturyAmbiguous:
			century++
		case DateInvalid:
			invalid++
		}
	}
	if absent > 0 {
		add(fmt.Sprintf("%d frame(s) have no LTC calendar date; absolute dates and cross-source order are unresolved for those segments", absent))
	}
	if century > 0 {
		add(fmt.Sprintf("%d frame(s) carry only a two-digit year; century-qualified dates are unresolved", century))
	}
	if invalid > 0 {
		add(fmt.Sprintf("%d frame(s) carry invalid ST 309 date bits", invalid))
	}
	for _, e := range t.Events {
		if e.Ambiguous {
			add(fmt.Sprintf("frame %d (%s): %s", e.FrameIndex, e.Timecode, e.Message))
		}
	}
	return out
}

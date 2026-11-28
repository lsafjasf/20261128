package ltc

import (
	"fmt"
	"time"
)

const (
	EventDuplicate           = "duplicate"
	EventRollback            = "rollback"
	EventForwardJump         = "forward-jump"
	EventMidnightRollover    = "midnight-rollover"
	EventMidnightIrregular   = "midnight-irregular"
	EventDropFrameChange     = "drop-frame-change"
	EventDateRollover        = "date-rollover"
	EventDateChange          = "date-change"
	EventDateJump            = "date-jump"
	EventDateRollback        = "date-rollback"
	EventDateNotAdvanced     = "date-not-advanced"
	EventDateWithoutMidnight = "date-change-without-midnight"
	EventInvalidFrame        = "invalid-frame"
)

func Analyze(label string, frames []Frame, nominalFPS int) Timeline {
	t := Timeline{
		Label:      label,
		NominalFPS: nominalFPS,
		FrameCount: len(frames),
		Frames:     make([]FrameObservation, 0, len(frames)),
	}
	if len(frames) == 0 {
		t.Ambiguities = append(t.Ambiguities, "source contains no LTC frames")
		return t
	}

	day := 0
	minDay, maxDay := 0, 0
	seen := make(map[string]int)
	var baseDate *time.Time
	segmentStart := 0
	segmentIndex := 0

	for i, f := range frames {
		obs := FrameObservation{
			Frame:        f,
			Index:        i,
			TimeAddress:  f.Timecode.DropFrameString(f.DropFrame),
			RelativeDay:  day,
			Chronology:   "first",
			SegmentIndex: segmentIndex,
		}
		fnum, ferr := FrameNumber(f.Timecode, nominalFPS, f.DropFrame)
		if ferr == nil {
			obs.FrameNumber = intPtr(fnum)
		}

		breakSegment := false
		if i > 0 {
			prev := &t.Frames[i-1]
			prevNum := prev.FrameNumber
			if ferr != nil || prevNum == nil {
				obs.Chronology = "invalid"
				obs.ChronologyAmbiguous = true
				addEvent(&t, EventInvalidFrame, i, obs.TimeAddress, i-1, prev.TimeAddress, nil, true,
					"cannot place a frame with an invalid time address on the timeline")
				breakSegment = true
			} else {
				pn := *prevNum
				fpd := FramesPerDay(nominalFPS, prev.DropFrame)
				expected := (pn + 1) % fpd
				exactMidnight := fnum == 0 && pn == fpd-1
				nearMidnight := fnum < nominalFPS*10 && pn >= fpd-nominalFPS*10 && fnum < pn

				if f.DropFrame != prev.DropFrame {
					obs.Chronology = EventDropFrameChange
					obs.ChronologyAmbiguous = true
					delta := fnum - pn
					addEvent(&t, EventDropFrameChange, i, obs.TimeAddress, i-1, prev.TimeAddress, &delta, true,
						"drop-frame flag changed; 29.97 and 30 fps counts are not directly comparable")
					breakSegment = true
				} else {
					switch {
					case exactMidnight:
						day++
						obs.Chronology = EventMidnightRollover
						addEvent(&t, EventMidnightRollover, i, obs.TimeAddress, i-1, prev.TimeAddress, intPtr(1), false,
							"time address crossed 24-hour midnight; the calendar date is only known if ST 309 date bits agree")
					case fnum == expected:
						obs.Chronology = "sequential"
					case fnum == pn:
						obs.Chronology = EventDuplicate
						obs.ChronologyAmbiguous = true
						addEvent(&t, EventDuplicate, i, obs.TimeAddress, i-1, prev.TimeAddress, intPtr(0), true,
							"repeated time address without forward frame advance")
						breakSegment = true
					case nearMidnight:
						day++
						obs.Chronology = EventMidnightIrregular
						obs.ChronologyAmbiguous = true
						delta := fnum - pn
						addEvent(&t, EventMidnightIrregular, i, obs.TimeAddress, i-1, prev.TimeAddress, &delta, true,
							"midnight-like wrap is not a clean 23:59:59:max -> 00:00:00:00 transition")
						breakSegment = true
					case fnum < pn:
						obs.Chronology = EventRollback
						obs.ChronologyAmbiguous = true
						delta := fnum - pn
						addEvent(&t, EventRollback, i, obs.TimeAddress, i-1, prev.TimeAddress, &delta, true,
							"time address moved backwards without a midnight rollover")
						breakSegment = true
					default:
						obs.Chronology = EventForwardJump
						obs.ChronologyAmbiguous = true
						delta := fnum - pn
						addEvent(&t, EventForwardJump, i, obs.TimeAddress, i-1, prev.TimeAddress, &delta, true,
							fmt.Sprintf("time address skipped %d frame(s)", delta-1))
						breakSegment = true
					}
				}
			}
		}

		if f.Date.Status == DateDetermined {
			if d, err := time.Parse("2006-01-02", f.Date.Date); err == nil {
				if baseDate == nil {
					base := d.AddDate(0, 0, -day)
					baseDate = &base
				}
				day = int(d.Sub(*baseDate).Hours() / 24)
				obs.RelativeDay = day
			}
		}

		if ferr == nil {
			key := fmt.Sprintf("%d|%s|%t", day, obs.TimeAddress, f.DropFrame)
			if prior, ok := seen[key]; ok && prior != i-1 && !obs.ChronologyAmbiguous {
				obs.Chronology = EventDuplicate
				obs.ChronologyAmbiguous = true
				addEvent(&t, EventDuplicate, i, obs.TimeAddress, prior, t.Frames[prior].TimeAddress, nil, true,
					fmt.Sprintf("time address was already seen at frame %d on inferred relative day %d", prior, day))
				breakSegment = true
			}
			seen[key] = i
		}

		dateKey := calendarKey(f.Date)
		if i > 0 && dateKey != "" {
			prev := &t.Frames[i-1]
			prevKey := calendarKey(prev.Date)
			if prevKey != "" && dateKey != prevKey {
				midnight := obs.Chronology == EventMidnightRollover
				relation, relationKnown := compareDates(prev.Date, f.Date)
				switch {
				case relationKnown && relation == 1 && midnight:
					addEvent(&t, EventDateRollover, i, obs.TimeAddress, i-1, prev.TimeAddress, nil, false,
						"ST 309 calendar date advanced by one day at midnight")
				case relationKnown && relation == 1 && !midnight:
					obs.ChronologyAmbiguous = true
					addEvent(&t, EventDateWithoutMidnight, i, obs.TimeAddress, i-1, prev.TimeAddress, nil, true,
						"ST 309 calendar date changed by one day without a 24-hour rollover")
					breakSegment = true
				case relationKnown && relation > 1:
					obs.ChronologyAmbiguous = true
					addEvent(&t, EventDateJump, i, obs.TimeAddress, i-1, prev.TimeAddress, nil, true,
						fmt.Sprintf("ST 309 calendar date jumped forward by %d days", relation))
					breakSegment = true
				case relationKnown && relation < 0:
					obs.ChronologyAmbiguous = true
					addEvent(&t, EventDateRollback, i, obs.TimeAddress, i-1, prev.TimeAddress, nil, true,
						"ST 309 calendar date moved backwards")
					breakSegment = true
				default:
					addEvent(&t, EventDateChange, i, obs.TimeAddress, i-1, prev.TimeAddress, nil, !midnight,
						"ST 309 calendar date changed; exact day delta cannot be established")
				}
			}
			if prevKey != "" && dateKey == prevKey && obs.Chronology == EventMidnightRollover {
				obs.ChronologyAmbiguous = true
				addEvent(&t, EventDateNotAdvanced, i, obs.TimeAddress, i-1, prev.TimeAddress, nil, true,
					"time address crossed midnight but the ST 309 date did not advance")
				breakSegment = true
			}
		}

		if len(f.Warnings) > 0 {
			for _, warning := range f.Warnings {
				addEvent(&t, EventInvalidFrame, i, obs.TimeAddress, -1, "", nil, true, warning)
			}
			obs.ChronologyAmbiguous = true
			breakSegment = true
		}
		if i > 0 && f.Date.Status != t.Frames[i-1].Date.Status {
			breakSegment = true
		}

		if breakSegment && i > 0 {
			finalizeSegment(&t, segmentStart, i-1, segmentIndex)
			segmentStart = i
			segmentIndex++
			obs.SegmentIndex = segmentIndex
		}
		obs.RelativeDay = day
		t.Frames = append(t.Frames, obs)
		if day < minDay {
			minDay = day
		}
		if day > maxDay {
			maxDay = day
		}
	}
	finalizeSegment(&t, segmentStart, len(t.Frames)-1, segmentIndex)
	t.RelativeDays = maxDay - minDay + 1
	t.Ambiguities = buildAmbiguities(t)
	return t
}

func intPtr(v int) *int {
	return &v
}

func addEvent(t *Timeline, typ string, index int, tc string, prevIndex int, prevTC string,
	delta *int, ambiguous bool, message string) {
	e := Event{Type: typ, FrameIndex: index, Timecode: tc, DeltaFrames: delta,
		Ambiguous: ambiguous, Message: message}
	if prevIndex >= 0 {
		e.PreviousIndex = intPtr(prevIndex)
		e.PreviousTimecode = prevTC
	}
	t.Events = append(t.Events, e)
}

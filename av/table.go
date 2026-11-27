package av

import "sort"

// EventKind distinguishes video and audio events.
type EventKind int

const (
	EventVideo EventKind = iota
	EventAudio
)

func (k EventKind) String() string {
	if k == EventVideo {
		return "V"
	}
	return "A"
}

// Event is one row of the event table, with times in milliseconds.
type Event struct {
	Kind      EventKind
	DecodeIdx int     // decode-order index within its track
	DTSms     float64 // decode timestamp
	CTSms     float64 // presentation timestamp (media timeline)
	DurMs     float64
	Sync      bool
}

// CutInfo aggregates everything known about one scene cut.
type CutInfo struct {
	Ordinal        int     // 1-based cut number
	VideoSampleIdx int     // decode index of the sync sample opening the new shot
	CutPresMs      float64 // presentation time of the cut (video media timeline)
	AudioJumpMs    float64 // audio PTS discontinuity straddling the cut
	AudioGapMs     float64 // uncovered audio presentation time within +-WindowMs of the cut
	VideoDTSJumpMs float64 // video DTS step at the cut minus one frame duration
	ElstShiftMs    float64 // video presentation shift implied by the edit list at the cut
	Window         []Event // presentation-order events near the cut
}

// WindowMs is the half-width of the per-cut inspection window.
const WindowMs = 150.0

// Discontinuity is an abnormal step in a track's decode timeline.
type Discontinuity struct {
	AfterIdx int     // decode index of the sample before the jump
	AtMs     float64 // timeline position of the sample after the jump
	JumpMs   float64 // step size minus the nominal sample duration
}

// EventTable holds decode-order and presentation-order views of the file.
type EventTable struct {
	VideoDecodeOrder []Event
	AudioDecodeOrder []Event
	Presentation     []Event // both tracks merged, sorted by presentation time
	AudioJumps       []Discontinuity
	VideoJumps       []Discontinuity
	Cuts             []CutInfo
}

// BuildEventTable constructs the full event table from the model.
func BuildEventTable(m *Model) *EventTable {
	t := &EventTable{}
	for _, s := range m.Video.Samples {
		t.VideoDecodeOrder = append(t.VideoDecodeOrder, Event{
			Kind: EventVideo, DecodeIdx: s.Index,
			DTSms: m.Video.Ms(s.DTS), CTSms: m.Video.Ms(s.CTS),
			DurMs: m.Video.Ms(s.Dur), Sync: s.Sync,
		})
	}
	for _, s := range m.Audio.Samples {
		t.AudioDecodeOrder = append(t.AudioDecodeOrder, Event{
			Kind: EventAudio, DecodeIdx: s.Index,
			DTSms: m.Audio.Ms(s.DTS), CTSms: m.Audio.Ms(s.CTS),
			DurMs: m.Audio.Ms(s.Dur), Sync: s.Sync,
		})
	}
	t.Presentation = append(append([]Event{}, t.VideoDecodeOrder...), t.AudioDecodeOrder...)
	sort.SliceStable(t.Presentation, func(i, j int) bool {
		if t.Presentation[i].CTSms != t.Presentation[j].CTSms {
			return t.Presentation[i].CTSms < t.Presentation[j].CTSms
		}
		return t.Presentation[i].Kind < t.Presentation[j].Kind
	})
	t.AudioJumps = findJumps(t.AudioDecodeOrder)
	t.VideoJumps = findJumps(t.VideoDecodeOrder)
	for ord, idx := range m.Cuts() {
		t.Cuts = append(t.Cuts, analyzeCut(m, ord+1, idx, t.AudioJumps))
	}
	return t
}

// findJumps returns the positions where the decode-timeline step exceeds the
// nominal sample duration by more than half a sample.
func findJumps(decodeOrder []Event) []Discontinuity {
	if len(decodeOrder) < 2 {
		return nil
	}
	nominal := decodeOrder[0].DurMs
	var out []Discontinuity
	for i := 1; i < len(decodeOrder); i++ {
		step := decodeOrder[i].DTSms - decodeOrder[i-1].DTSms
		if step-nominal > nominal/2 {
			out = append(out, Discontinuity{
				AfterIdx: decodeOrder[i-1].DecodeIdx,
				AtMs:     decodeOrder[i].DTSms,
				JumpMs:   step - nominal,
			})
		}
	}
	return out
}

// elstShiftAt returns the video presentation shift (movie time minus media
// presentation time) implied by the edit list at media time cts, in ticks.
// Within one edit-list entry the shift is constant: movieBase-(mt-minCTS).
func elstShiftAt(m *Model, cts int64) int64 {
	if len(m.Video.Elst) == 0 {
		return 0
	}
	minCTS := m.Video.Samples[0].CTS
	for _, s := range m.Video.Samples {
		if s.CTS < minCTS {
			minCTS = s.CTS
		}
	}
	var movieBase int64 // accumulated movie time of earlier segments, track ticks
	for _, e := range m.Video.Elst {
		segTicks := e.SegmentDuration * m.Video.Timescale / m.MovieTimescale
		if e.MediaTime < 0 {
			// Empty edit: movie time advances, media does not.
			movieBase += segTicks
			continue
		}
		if cts >= e.MediaTime && cts < e.MediaTime+segTicks {
			return movieBase - (e.MediaTime - minCTS)
		}
		movieBase += segTicks
	}
	return 0
}

func analyzeCut(m *Model, ordinal, vidx int, audioJumps []Discontinuity) CutInfo {
	ci := CutInfo{Ordinal: ordinal, VideoSampleIdx: vidx}
	vs := m.Video.Samples[vidx]
	ci.CutPresMs = m.Video.Ms(vs.CTS)

	// Video DTS step at the cut (decode order is file order).
	if vidx > 0 {
		step := vs.DTS - m.Video.Samples[vidx-1].DTS
		ci.VideoDTSJumpMs = m.Video.Ms(step - m.FrameDur())
	}

	// Nearest audio discontinuity to this cut (reorder-wait holes drift
	// late by the accumulated per-cut delay, so allow for that here).
	cutMs := ci.CutPresMs
	best := -1.0
	for _, j := range audioJumps {
		d := j.AtMs - cutMs
		if d < 0 {
			d = -d
		}
		if best < 0 || d < best {
			best = d
			ci.AudioJumpMs = j.JumpMs
		}
	}

	// Uncovered audio presentation time inside the window.
	lo, hi := cutMs-WindowMs, cutMs+WindowMs
	cursor := lo
	for _, s := range m.Audio.Samples {
		start := m.Audio.Ms(s.CTS)
		end := m.Audio.Ms(s.CTS + s.Dur)
		if end <= cursor {
			continue
		}
		if start > hi {
			break
		}
		if start > cursor {
			ci.AudioGapMs += start - cursor
		}
		if end > cursor {
			cursor = end
		}
	}
	if cursor < hi {
		ci.AudioGapMs += hi - cursor
	}

	ci.ElstShiftMs = m.Video.Ms(elstShiftAt(m, vs.CTS))

	// Presentation-order events inside the window.
	var evs []Event
	add := func(kind EventKind, tm *TrackModel) {
		for _, s := range tm.Samples {
			c := tm.Ms(s.CTS)
			if c >= lo && c <= hi {
				evs = append(evs, Event{
					Kind: kind, DecodeIdx: s.Index,
					DTSms: tm.Ms(s.DTS), CTSms: c,
					DurMs: tm.Ms(s.Dur), Sync: s.Sync,
				})
			}
		}
	}
	add(EventVideo, m.Video)
	add(EventAudio, m.Audio)
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].CTSms != evs[j].CTSms {
			return evs[i].CTSms < evs[j].CTSms
		}
		return evs[i].Kind < evs[j].Kind
	})
	ci.Window = evs
	return ci
}

package amrwb

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrInvalidFrame     = errors.New("invalid AMR-WB frame")
	ErrOutOfOrder       = errors.New("AMR-WB frame index is out of order")
	ErrTimelineOverflow = errors.New("AMR-WB sample timeline overflow")
)

// Disposition describes how one 20 ms slot was represented in the timeline.
type Disposition uint8

const (
	// DispositionGood is a received, usable frame.
	DispositionGood Disposition = iota
	// DispositionBad is a received frame with Q=0. It still contributes one
	// 320-sample slot and is concealed without shifting later indices.
	DispositionBad
	// DispositionLost is a real gap or an explicit SPEECH_LOST frame.
	DispositionLost
	// DispositionNoData is an intentional no-transmission/DTX slot.
	DispositionNoData
)

func (d Disposition) String() string {
	switch d {
	case DispositionGood:
		return "good"
	case DispositionBad:
		return "bad"
	case DispositionLost:
		return "lost"
	case DispositionNoData:
		return "no_data"
	default:
		return fmt.Sprintf("disposition(%d)", uint8(d))
	}
}

// Frame identifies one 20 ms AMR-WB frame. Index is the monotonic frame-block
// index, not a byte or sample offset. Size is the total octet-aligned frame
// size including its one-octet header.
type Frame struct {
	Index   int64
	Mode    Mode
	Size    int
	Quality FrameQuality
}

// SampleSpan is the resulting timeline entry for one frame slot.
// SampleEnd is exclusive. Each span always contains SamplesPerFrame samples,
// independent of Mode and Size.
type SampleSpan struct {
	Index       int64
	SampleStart uint64
	SampleEnd   uint64
	Samples     int
	Mode        Mode
	Size        int
	Quality     FrameQuality
	Disposition Disposition
	Concealed   bool
	ModeChanged bool
}

// Stats summarizes a timeline. FrameSlots and Samples describe time slots,
// while Bytes counts only frame bytes that were actually present in the input.
type Stats struct {
	FrameSlots      int
	Samples         uint64
	GoodFrames      int
	BadFrames       int
	LostFrames      int
	NoDataFrames    int
	ConcealedFrames int
	ModeChanges     int
	Bytes           uint64
}

// HeaderSamples is the sample duration accumulated from the frame slots.
// For a valid timeline it equals Samples.
func (s Stats) HeaderSamples() uint64 {
	return uint64(s.FrameSlots) * SamplesPerFrame
}

// Consistent reports whether the produced sample total matches the cumulative
// duration of frame headers.
func (s Stats) Consistent() bool {
	return s.Samples == s.HeaderSamples()
}

// Timeline converts frame metadata into a contiguous sample timeline. It
// never derives sample duration from encoded frame size or mode.
type Timeline struct {
	nextIndex   int64
	nextSample  uint64
	started     bool
	lastMode    Mode
	hasLastMode bool
	spans       []SampleSpan
	stats       Stats
}

// NewTimeline returns an empty AMR-WB sample timeline.
func NewTimeline() *Timeline {
	return &Timeline{}
}

// BuildTimeline builds a timeline from frames in order.
func BuildTimeline(frames []Frame) (*Timeline, error) {
	timeline := NewTimeline()
	if err := timeline.AddAll(frames); err != nil {
		return nil, err
	}
	return timeline, nil
}

// Add appends a frame to the timeline. A non-consecutive Index represents real
// frame loss and creates concealed slots between the previous frame and f.
// A bad frame consumes only its own slot, so later sample starts remain
// contiguous.
func (t *Timeline) Add(f Frame) (SampleSpan, error) {
	if !f.Mode.Valid() {
		return SampleSpan{}, fmt.Errorf("%w: reserved mode %d", ErrInvalidFrame, uint8(f.Mode))
	}
	if f.Index < 0 {
		return SampleSpan{}, fmt.Errorf("%w: negative index %d", ErrInvalidFrame, f.Index)
	}
	if want := f.Mode.FrameOctets(); f.Size != want {
		return SampleSpan{}, fmt.Errorf(
			"%w: mode %d requires %d octets including header, got %d",
			ErrInvalidFrame, uint8(f.Mode), want, f.Size,
		)
	}
	if t.started && f.Index < t.nextIndex {
		return SampleSpan{}, fmt.Errorf(
			"%w: got frame %d after %d", ErrOutOfOrder, f.Index, t.nextIndex,
		)
	}

	missing := int64(0)
	if t.started && f.Index > t.nextIndex {
		missing = f.Index - t.nextIndex
	}
	if err := t.checkSampleRoom(missing + 1); err != nil {
		return SampleSpan{}, err
	}

	for i := int64(0); i < missing; i++ {
		t.appendSpan(SampleSpan{
			Index:       t.nextIndex,
			SampleStart: t.nextSample,
			SampleEnd:   t.nextSample + SamplesPerFrame,
			Samples:     SamplesPerFrame,
			Mode:        ModeSpeechLost,
			Disposition: DispositionLost,
			Concealed:   true,
		})
	}

	disposition := DispositionGood
	concealed := false
	switch {
	case f.Quality == QualityBad:
		disposition = DispositionBad
		concealed = true
	case f.Mode == ModeSpeechLost:
		disposition = DispositionLost
		concealed = true
	case f.Mode == ModeNoData:
		disposition = DispositionNoData
		concealed = true
	}

	modeChanged := t.hasLastMode && f.Mode.Valid() && f.Mode != ModeSpeechLost &&
		f.Mode != ModeNoData && f.Mode != t.lastMode
	if f.Mode != ModeSpeechLost && f.Mode != ModeNoData {
		t.lastMode = f.Mode
		t.hasLastMode = true
	}

	span := SampleSpan{
		Index:       f.Index,
		SampleStart: t.nextSample,
		SampleEnd:   t.nextSample + SamplesPerFrame,
		Samples:     SamplesPerFrame,
		Mode:        f.Mode,
		Size:        f.Size,
		Quality:     f.Quality,
		Disposition: disposition,
		Concealed:   concealed,
		ModeChanged: modeChanged,
	}
	t.appendSpan(span)
	return span, nil
}

// AddAll adds frames in order and returns the first error.
func (t *Timeline) AddAll(frames []Frame) error {
	for _, f := range frames {
		if _, err := t.Add(f); err != nil {
			return err
		}
	}
	return nil
}

// Spans returns a copy of the timeline entries.
func (t *Timeline) Spans() []SampleSpan {
	out := make([]SampleSpan, len(t.spans))
	copy(out, t.spans)
	return out
}

// Stats returns a snapshot of counters and totals.
func (t *Timeline) Stats() Stats {
	return t.stats
}

func (t *Timeline) checkSampleRoom(slots int64) error {
	if slots < 0 || slots > math.MaxUint64/SamplesPerFrame {
		return ErrTimelineOverflow
	}
	samples := uint64(slots) * SamplesPerFrame
	if t.nextSample > math.MaxUint64-samples {
		return ErrTimelineOverflow
	}
	return nil
}

func (t *Timeline) appendSpan(s SampleSpan) {
	t.spans = append(t.spans, s)
	t.nextIndex = s.Index + 1
	t.nextSample = s.SampleEnd
	t.started = true

	t.stats.FrameSlots++
	t.stats.Samples = t.nextSample
	t.stats.Bytes += uint64(s.Size)
	switch s.Disposition {
	case DispositionGood:
		t.stats.GoodFrames++
	case DispositionBad:
		t.stats.BadFrames++
	case DispositionLost:
		t.stats.LostFrames++
	case DispositionNoData:
		t.stats.NoDataFrames++
	}
	if s.Concealed {
		t.stats.ConcealedFrames++
	}
	if s.ModeChanged {
		t.stats.ModeChanges++
	}
}

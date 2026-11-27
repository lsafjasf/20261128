package lc3

import "fmt"

// Kind distinguishes decoded audio frames from the silent/PLC spans that fill
// lost-packet holes.
type Kind uint8

const (
	// KindFrame is a complete, received LC3 frame.
	KindFrame Kind = iota
	// KindGap is a span that contains no received frame because one or more
	// packets were lost.
	KindGap
)

// Segment is one contiguous run on the reconstructed PCM timeline. A frame
// segment covers exactly Samples(rate, Dur) samples; a gap segment covers the
// samples that the missing frame(s) would have occupied.
type Segment struct {
	Kind     Kind
	Start    uint64 // absolute PCM sample index of the first sample (inclusive)
	End      uint64 // absolute PCM sample index immediately after the last sample
	Duration Duration
	Rate     uint32
	// Seq is the 16-bit RTP sequence number for a frame, or the sequence
	// number that the first missing frame would have carried for a gap.
	Seq uint16
	// Lost is the number of consecutive missing frames represented by a gap
	// segment; it is 0 for KindFrame.
	Lost int
	// Octets is the encoded payload size in bytes for a frame, or 0 for a gap.
	Octets int
}

// Length returns the number of PCM samples the segment occupies.
func (s Segment) Length() uint64 { return s.End - s.Start }

// Frame is a decoded LC3 frame together with everything needed to place it on
// the sample timeline.
type Frame struct {
	Seq      uint16
	Duration Duration
	Octets   int
	Payload  []byte
	// Truncated is true when the framing declared Octets bytes but the input
	// buffer ended early. Payload then holds whatever bytes were available.
	Truncated bool
}

// Timeline is the fully reconstructed, gap-aware PCM schedule for a stream.
type Timeline struct {
	Rate     uint32
	Segments []Segment
	// Received and Lost count decoded and missing LC3 frames respectively.
	Received int
	Lost     int
	// TailTruncated marks a final frame whose encoded payload was cut short.
	TailTruncated bool
}

// TotalSamples is the absolute PCM length of the whole stream, gaps included.
func (t Timeline) TotalSamples() uint64 {
	if len(t.Segments) == 0 {
		return 0
	}
	return t.Segments[len(t.Segments)-1].End
}

// Frames returns only the received (non-gap) segments, in timeline order.
func (t Timeline) Frames() []Segment {
	out := make([]Segment, 0, t.Received)
	for _, seg := range t.Segments {
		if seg.Kind == KindFrame {
			out = append(out, seg)
		}
	}
	return out
}

// Gaps returns only the loss spans, in timeline order.
func (t Timeline) Gaps() []Segment {
	out := make([]Segment, 0)
	for _, seg := range t.Segments {
		if seg.Kind == KindGap {
			out = append(out, seg)
		}
	}
	return out
}

// seqDiff computes the signed forward distance from a to b in 16-bit sequence
// number space. Positive values mean b is later than a.
func seqDiff(a, b uint16) int {
	return int(int16(b - a))
}

// GapPolicy controls how the PCM length of a lost-frame span is estimated. The
// exact duration of a lost frame cannot be recovered, so callers choose a
// convention; the number of missing frames (Segment.Lost) is always exact.
type GapPolicy int

const (
	// GapUseResumingDuration sizes each missing frame with the duration of the
	// first frame received after the hole. This tracks a mode switch boundary
	// well and is the default used by Build.
	GapUseResumingDuration GapPolicy = iota
	// GapUseFixedDuration sizes every missing frame with a caller-supplied
	// duration, useful when the sender's mode is known out-of-band.
	GapUseFixedDuration
)

// Options tunes timeline reconstruction.
type Options struct {
	// Gap selects the convention for sizing loss spans.
	Gap GapPolicy
	// FixedGapDuration is the per-missing-frame duration when Gap is
	// GapUseFixedDuration; ignored otherwise.
	FixedGapDuration Duration
}

// Build reconstructs the timeline from frames that are already ordered by
// arrival on the wire. The first frame anchors sample 0. Holes are detected
// from the 16-bit sequence numbers and filled with gap segments. Each frame's
// own duration determines its sample length, so 10 ms <-> 7.5 ms switching is
// handled without any fixed-duration assumption.
func Build(rate uint32, frames []Frame) (Timeline, error) {
	return BuildWithOptions(rate, frames, Options{Gap: GapUseResumingDuration})
}

// BuildWithOptions is Build with an explicit gap-sizing policy.
func BuildWithOptions(rate uint32, frames []Frame, opts Options) (Timeline, error) {
	if !SupportedRate(rate) {
		return Timeline{}, fmt.Errorf("lc3: unsupported sample rate %d Hz", rate)
	}
	if opts.Gap == GapUseFixedDuration && !opts.FixedGapDuration.Valid() {
		return Timeline{}, fmt.Errorf("lc3: FixedGapDuration must be valid for GapUseFixedDuration")
	}
	tl := Timeline{Rate: rate, Segments: make([]Segment, 0, len(frames))}

	var cursor uint64
	var expected uint16
	for i := range frames {
		f := frames[i]
		if !f.Duration.Valid() {
			return Timeline{}, fmt.Errorf("lc3: frame seq %d has invalid duration", f.Seq)
		}
		n, err := Samples(rate, f.Duration)
		if err != nil {
			return Timeline{}, err
		}

		if i > 0 {
			jump := seqDiff(expected, f.Seq)
			if jump < 0 {
				return Timeline{}, fmt.Errorf("lc3: out-of-order or duplicate frame seq %d (expected %d)", f.Seq, expected)
			}
			if jump > 0 {
				gapDur, gapN := f.Duration, n
				if opts.Gap == GapUseFixedDuration {
					gapDur = opts.FixedGapDuration
					gapN, err = Samples(rate, gapDur)
					if err != nil {
						return Timeline{}, err
					}
				}
				tl.appendGap(cursor, expected, jump, gapDur, rate, gapN)
				cursor = tl.Segments[len(tl.Segments)-1].End
			}
		}

		tl.Segments = append(tl.Segments, Segment{
			Kind:     KindFrame,
			Start:    cursor,
			End:      cursor + uint64(n),
			Duration: f.Duration,
			Rate:     rate,
			Seq:      f.Seq,
			Octets:   f.Octets,
		})
		cursor += uint64(n)
		expected = f.Seq + 1
		tl.Received++
		if f.Truncated {
			tl.TailTruncated = true
		}
	}
	return tl, nil
}

// appendGap records the missing frames starting at sequence number firstSeq.
// The exact per-frame length of a lost frame is not knowable, so the gap is
// estimated using the duration of the frame that resumes the stream (a common
// PLC convention), while recording the frame count explicitly.
func (t *Timeline) appendGap(cursor uint64, firstSeq uint16, count int, dur Duration, rate uint32, perFrame int) {
	t.Segments = append(t.Segments, Segment{
		Kind:     KindGap,
		Start:    cursor,
		End:      cursor + uint64(perFrame*count),
		Duration: dur,
		Rate:     rate,
		Seq:      firstSeq,
		Lost:     count,
	})
	t.Lost += count
}

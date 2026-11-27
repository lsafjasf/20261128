package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Frame is one exported image with its sequence number parsed from the
// file name.
type Frame struct {
	Name string
	Seq  int
}

// Gap describes a run of missing sequence numbers between two exported
// frames.
type Gap struct {
	AfterSeq int   // last exported frame before the gap
	Missing  []int // sequence numbers that were never exported
}

// Len returns how many frames are missing in the gap.
func (g Gap) Len() int { return len(g.Missing) }

var defaultSeqPattern = regexp.MustCompile(`(\d+)`)

// ParseSeq extracts the sequence number from a file name. When pattern is
// empty, the last run of digits in the name is used; otherwise the first
// capture group of the pattern is used.
func ParseSeq(name, pattern string) (int, error) {
	if pattern == "" {
		matches := defaultSeqPattern.FindAllString(name, -1)
		if len(matches) == 0 {
			return 0, fmt.Errorf("no digits in %q", name)
		}
		return strconv.Atoi(matches[len(matches)-1])
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return 0, fmt.Errorf("bad pattern: %w", err)
	}
	m := re.FindStringSubmatch(name)
	if len(m) < 2 {
		return 0, fmt.Errorf("pattern did not match %q", name)
	}
	return strconv.Atoi(m[1])
}

// CollectFrames parses and sorts frames by sequence number. Duplicate
// sequence numbers are rejected because they would make the timeline
// ambiguous.
func CollectFrames(names []string, pattern string) ([]Frame, error) {
	frames := make([]Frame, 0, len(names))
	seen := make(map[int]string, len(names))
	for _, name := range names {
		seq, err := ParseSeq(name, pattern)
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[seq]; dup {
			return nil, fmt.Errorf("duplicate sequence %d: %q and %q", seq, prev, name)
		}
		seen[seq] = name
		frames = append(frames, Frame{Name: name, Seq: seq})
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i].Seq < frames[j].Seq })
	return frames, nil
}

// FindGaps returns every run of missing sequence numbers in sorted frames.
func FindGaps(frames []Frame) []Gap {
	var gaps []Gap
	for i := 1; i < len(frames); i++ {
		prev, cur := frames[i-1].Seq, frames[i].Seq
		if cur-prev <= 1 {
			continue
		}
		g := Gap{AfterSeq: prev}
		for n := prev + 1; n < cur; n++ {
			g.Missing = append(g.Missing, n)
		}
		gaps = append(gaps, g)
	}
	return gaps
}

// Reference anchors sequence numbers to wall-clock time. FPS is the export
// frame rate; the real duration of the sequence is derived from sequence
// numbers, never from the number of exported files.
type Reference struct {
	AnchorSeq  int
	AnchorTime time.Time
	FPS        float64
}

// NewReference builds a Reference from two timestamped frames, deriving the
// effective frame rate from them.
func NewReference(seqA int, timeA time.Time, seqB int, timeB time.Time) (Reference, error) {
	if seqA == seqB {
		return Reference{}, fmt.Errorf("reference frames must have different sequence numbers")
	}
	fps := float64(seqB-seqA) / timeB.Sub(timeA).Seconds()
	if fps <= 0 {
		return Reference{}, fmt.Errorf("non-positive frame rate from references")
	}
	return Reference{AnchorSeq: seqA, AnchorTime: timeA, FPS: fps}, nil
}

// TimeOf returns the wall-clock time of a sequence number.
func (r Reference) TimeOf(seq int) time.Time {
	offset := time.Duration(float64(seq-r.AnchorSeq) / r.FPS * float64(time.Second))
	return r.AnchorTime.Add(offset)
}

// Duration returns the real span covered by the sequence numbers, including
// any gaps. It deliberately ignores how many files were exported.
func (r Reference) Duration(frames []Frame) time.Duration {
	if len(frames) == 0 {
		return 0
	}
	span := frames[len(frames)-1].Seq - frames[0].Seq
	return time.Duration(float64(span) / r.FPS * float64(time.Second))
}

// Candidate is a missing frame with the wall-clock time it should have had.
type Candidate struct {
	Seq  int
	Time time.Time
}

// FillCandidates lists the frames needed to close a gap, each with its
// expected timestamp on the real timeline.
func FillCandidates(r Reference, g Gap) []Candidate {
	cands := make([]Candidate, 0, len(g.Missing))
	for _, seq := range g.Missing {
		cands = append(cands, Candidate{Seq: seq, Time: r.TimeOf(seq)})
	}
	return cands
}

// FormatTimeline renders one line per frame: sequence, name, and the real
// timestamp, with gap markers inserted where frames are missing.
func FormatTimeline(r Reference, frames []Frame) string {
	var b strings.Builder
	gaps := FindGaps(frames)
	gapAfter := make(map[int]Gap, len(gaps))
	for _, g := range gaps {
		gapAfter[g.AfterSeq] = g
	}
	for _, f := range frames {
		fmt.Fprintf(&b, "#%06d  %-24s  %s\n", f.Seq, f.Name, r.TimeOf(f.Seq).Format(time.RFC3339Nano))
		if g, ok := gapAfter[f.Seq]; ok {
			fmt.Fprintf(&b, "  ... gap: %d frame(s) missing (%d-%d)\n", g.Len(), g.Missing[0], g.Missing[len(g.Missing)-1])
		}
	}
	return b.String()
}

package handle

// Cue is a subtitle entry in source time, ms.
type Cue struct {
	Start int64
	End   int64
	Text  string
}

// AudioTrack is an attached audio segment in source time, ms.
type AudioTrack struct {
	// SourceStart is where the audio begins in source time.
	SourceStart int64
	Duration    int64
}

// PlacedCue is a cue re-based to the exported timeline (0 = ExportStart).
type PlacedCue struct {
	Cue
	// Offset is the cue start relative to the exported timeline, ms.
	Offset int64
}

// PlacedAudio is an audio track re-based to the exported timeline.
type PlacedAudio struct {
	AudioTrack
	// Offset is where the audio starts on the exported timeline, ms.
	// Negative means the audio begins before ExportStart and must be
	// trimmed by -Offset at its head.
	Offset int64
	// TrimHead is how much to cut from the audio head, ms (>= 0).
	TrimHead int64
}

// Layout is the result of syncing subtitles and audio to a RangePlan.
type Layout struct {
	Plan  RangePlan
	Cues  []PlacedCue
	Audio []PlacedAudio
}

// Relayout re-bases cues and audio onto the plan's exported timeline.
// Cues fully outside [ExportStart, ExportEnd) are dropped; partially
// overlapping cues are clamped. Call it again after any range change.
func Relayout(p RangePlan, cues []Cue, tracks []AudioTrack) Layout {
	l := Layout{Plan: p}
	for _, c := range cues {
		start := max64(c.Start, p.ExportStart)
		end := min64(c.End, p.ExportEnd)
		if end <= start {
			continue
		}
		l.Cues = append(l.Cues, PlacedCue{
			Cue:    Cue{Start: start, End: end, Text: c.Text},
			Offset: start - p.ExportStart,
		})
	}
	for _, t := range tracks {
		trim := max64(0, p.ExportStart-t.SourceStart)
		if trim >= t.Duration {
			continue // track ends before the export window
		}
		l.Audio = append(l.Audio, PlacedAudio{
			AudioTrack: t,
			Offset:     t.SourceStart + trim - p.ExportStart,
			TrimHead:   trim,
		})
	}
	return l
}

// Adjust moves the cut range and returns the re-synced layout in one
// step. It validates the new range against the same source duration.
func Adjust(p RangePlan, newIn, newOut int64, cues []Cue, tracks []AudioTrack) (Layout, error) {
	np, err := Plan(p.SourceDuration, newIn, newOut, p.Handle)
	if err != nil {
		return Layout{}, err
	}
	return Relayout(np, cues, tracks), nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

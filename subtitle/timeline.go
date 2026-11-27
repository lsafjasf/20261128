package subtitle

import (
	"fmt"
	"sort"
	"time"
)

// Timeline is an immutable chapter and cue schedule. Chapter ranges may
// overlap; the timeline never resolves such an overlap on its own.
type Timeline struct {
	duration time.Duration
	chapters []Chapter
	cues     []Cue
	byID     map[string]Chapter
}

// NewTimeline validates and copies the schedule. A non-positive duration means
// that the media length is unknown; a positive duration bounds every range.
func NewTimeline(duration time.Duration, chapters []Chapter, cues []Cue) (*Timeline, error) {
	if duration < 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidDuration, duration)
	}

	chapterCopy := cloneChapters(chapters)
	byID := make(map[string]Chapter, len(chapterCopy))
	for i, chapter := range chapterCopy {
		if chapter.ID == "" {
			return nil, fmt.Errorf("%w: chapter at index %d", ErrMissingID, i)
		}
		if !chapter.Range.Valid() {
			return nil, fmt.Errorf("%w: chapter %q has %s", ErrInvalidRange, chapter.ID, chapter.Range)
		}
		if duration > 0 && chapter.Range.End > duration {
			return nil, fmt.Errorf("%w: chapter %q ends at %s, duration %s",
				ErrOutOfBounds, chapter.ID, chapter.Range.End, duration)
		}
		if _, exists := byID[chapter.ID]; exists {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateChapter, chapter.ID)
		}
		byID[chapter.ID] = chapter
	}

	cueCopy := cloneCues(cues)
	seenCue := make(map[string]struct{}, len(cueCopy))
	for i, cue := range cueCopy {
		if cue.ID == "" {
			return nil, fmt.Errorf("%w: cue at index %d", ErrMissingID, i)
		}
		if !cue.Range.Valid() {
			return nil, fmt.Errorf("%w: cue %q has %s", ErrInvalidRange, cue.ID, cue.Range)
		}
		if duration > 0 && cue.Range.End > duration {
			return nil, fmt.Errorf("%w: cue %q ends at %s, duration %s",
				ErrOutOfBounds, cue.ID, cue.Range.End, duration)
		}
		if _, exists := seenCue[cue.ID]; exists {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateCue, cue.ID)
		}
		seenCue[cue.ID] = struct{}{}
		if cue.ChapterID != "" {
			if _, exists := byID[cue.ChapterID]; !exists {
				return nil, fmt.Errorf("%w: cue %q -> %q",
					ErrUnknownChapter, cue.ID, cue.ChapterID)
			}
		}
	}

	sort.SliceStable(chapterCopy, func(i, j int) bool {
		return chapterLess(chapterCopy[i], chapterCopy[j])
	})
	sort.SliceStable(cueCopy, func(i, j int) bool {
		return cueLess(cueCopy[i], cueCopy[j])
	})

	return &Timeline{
		duration: duration,
		chapters: chapterCopy,
		cues:     cueCopy,
		byID:     byID,
	}, nil
}

// Duration returns the media duration, or zero when it is unknown.
func (t *Timeline) Duration() time.Duration {
	if t == nil {
		return 0
	}
	return t.duration
}

// Chapters returns a copy of all chapters in boundary order.
func (t *Timeline) Chapters() []Chapter {
	if t == nil {
		return nil
	}
	return cloneChapters(t.chapters)
}

// Cues returns a copy of all cues in time order.
func (t *Timeline) Cues() []Cue {
	if t == nil {
		return nil
	}
	return cloneCues(t.cues)
}

// Chapter looks up a chapter by stable ID.
func (t *Timeline) Chapter(id string) (Chapter, bool) {
	if t == nil {
		return Chapter{}, false
	}
	chapter, ok := t.byID[id]
	return chapter, ok
}

// ChaptersAt returns every chapter covering at. More than one result is a
// genuine ambiguity and is left for the caller to resolve manually.
func (t *Timeline) ChaptersAt(at time.Duration) []Chapter {
	if t == nil {
		return nil
	}
	var candidates []Chapter
	for _, chapter := range t.chapters {
		if chapter.Range.Contains(at) {
			candidates = append(candidates, chapter)
		}
	}
	return candidates
}

// CuesAt returns every subtitle cue covering at, independent of chapter.
func (t *Timeline) CuesAt(at time.Duration) []Cue {
	if t == nil {
		return nil
	}
	var active []Cue
	for _, cue := range t.cues {
		if cue.Range.Contains(at) {
			active = append(active, cue)
		}
	}
	return active
}

// CuesForChapterAt returns timeline-wide cues plus cues owned by chapterID.
// Unknown or empty chapter IDs only receive timeline-wide cues.
func (t *Timeline) CuesForChapterAt(at time.Duration, chapterID string) []Cue {
	if t == nil {
		return nil
	}
	var active []Cue
	for _, cue := range t.cues {
		if !cue.Range.Contains(at) {
			continue
		}
		if cue.ChapterID == "" || cue.ChapterID == chapterID {
			active = append(active, cue)
		}
	}
	return active
}

// Boundaries returns every distinct chapter start and end in ascending order.
// It is intended for building chapter markers and boundary-aware seeking.
func (t *Timeline) Boundaries() []time.Duration {
	if t == nil {
		return nil
	}
	seen := make(map[time.Duration]struct{}, len(t.chapters)*2)
	for _, chapter := range t.chapters {
		seen[chapter.Range.Start] = struct{}{}
		seen[chapter.Range.End] = struct{}{}
	}
	boundaries := make([]time.Duration, 0, len(seen))
	for at := range seen {
		boundaries = append(boundaries, at)
	}
	sort.Slice(boundaries, func(i, j int) bool { return boundaries[i] < boundaries[j] })
	return boundaries
}

func chapterLess(a, b Chapter) bool {
	if a.Range.Start != b.Range.Start {
		return a.Range.Start < b.Range.Start
	}
	if a.Range.End != b.Range.End {
		return a.Range.End < b.Range.End
	}
	return a.ID < b.ID
}

func cueLess(a, b Cue) bool {
	if a.Range.Start != b.Range.Start {
		return a.Range.Start < b.Range.Start
	}
	if a.Range.End != b.Range.End {
		return a.Range.End < b.Range.End
	}
	return a.ID < b.ID
}

func cloneChapters(in []Chapter) []Chapter {
	if len(in) == 0 {
		return nil
	}
	out := make([]Chapter, len(in))
	copy(out, in)
	return out
}

func cloneCues(in []Cue) []Cue {
	if len(in) == 0 {
		return nil
	}
	out := make([]Cue, len(in))
	copy(out, in)
	return out
}

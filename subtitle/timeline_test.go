package subtitle

import (
	"errors"
	"testing"
	"time"
)

func mustTimeline(t *testing.T, duration time.Duration, chapters []Chapter, cues []Cue) *Timeline {
	t.Helper()
	timeline, err := NewTimeline(duration, chapters, cues)
	if err != nil {
		t.Fatalf("NewTimeline: %v", err)
	}
	return timeline
}

func TestTimelineValidation(t *testing.T) {
	sec := time.Second
	validChapter := Chapter{ID: "a", Range: Range{Start: 0, End: 10 * sec}}
	validCue := Cue{ID: "c", Range: Range{Start: 1 * sec, End: 2 * sec}, Text: "x"}

	tests := []struct {
		name     string
		duration time.Duration
		chapters []Chapter
		cues     []Cue
		want     error
	}{
		{
			name:     "negative duration",
			duration: -sec,
			want:     ErrInvalidDuration,
		},
		{
			name:     "missing chapter id",
			chapters: []Chapter{{Range: validChapter.Range}},
			want:     ErrMissingID,
		},
		{
			name:     "empty chapter range",
			chapters: []Chapter{{ID: "a", Range: Range{Start: sec, End: sec}}},
			want:     ErrInvalidRange,
		},
		{
			name:     "chapter beyond duration",
			duration: 5 * sec,
			chapters: []Chapter{validChapter},
			want:     ErrOutOfBounds,
		},
		{
			name:     "duplicate chapter id",
			chapters: []Chapter{validChapter, validChapter},
			want:     ErrDuplicateChapter,
		},
		{
			name:     "missing cue id",
			chapters: []Chapter{validChapter},
			cues:     []Cue{{Range: validCue.Range}},
			want:     ErrMissingID,
		},
		{
			name:     "invalid cue range",
			chapters: []Chapter{validChapter},
			cues:     []Cue{{ID: "c", Range: Range{Start: 2 * sec, End: sec}}},
			want:     ErrInvalidRange,
		},
		{
			name:     "duplicate cue id",
			chapters: []Chapter{validChapter},
			cues:     []Cue{validCue, validCue},
			want:     ErrDuplicateCue,
		},
		{
			name:     "cue references unknown chapter",
			chapters: []Chapter{validChapter},
			cues:     []Cue{{ID: "c", ChapterID: "missing", Range: validCue.Range}},
			want:     ErrUnknownChapter,
		},
		{
			name:     "valid overlapping chapters",
			chapters: []Chapter{validChapter, {ID: "b", Range: Range{Start: 5 * sec, End: 15 * sec}}},
			cues:     []Cue{validCue},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewTimeline(test.duration, test.chapters, test.cues)
			if test.want == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestChapterBoundariesAreHalfOpen(t *testing.T) {
	sec := time.Second
	timeline := mustTimeline(t, 30*sec, []Chapter{
		{ID: "two", Range: Range{Start: 10 * sec, End: 20 * sec}},
		{ID: "three", Range: Range{Start: 20 * sec, End: 30 * sec}},
	}, nil)

	if got := timeline.ChaptersAt(20 * sec); len(got) != 1 || got[0].ID != "three" {
		t.Fatalf("position 20s should belong only to chapter three, got %+v", got)
	}
	candidates := timeline.ChaptersAt(19*sec + 999*time.Millisecond)
	if len(candidates) != 1 || candidates[0].ID != "two" {
		t.Fatalf("position just before 20s should belong to chapter two, got %+v", candidates)
	}
}

func TestOverlappingChaptersRemainCandidates(t *testing.T) {
	sec := time.Second
	timeline := mustTimeline(t, 40*sec, []Chapter{
		{ID: "late-eight", Range: Range{Start: 10 * sec, End: 30 * sec}},
		{ID: "early-two", Range: Range{Start: 20 * sec, End: 40 * sec}},
	}, nil)

	got := timeline.ChaptersAt(25 * sec)
	if len(got) != 2 {
		t.Fatalf("expected two candidates at overlap, got %+v", got)
	}
	boundaries := timeline.Boundaries()
	want := []time.Duration{10 * sec, 20 * sec, 30 * sec, 40 * sec}
	if len(boundaries) != len(want) {
		t.Fatalf("boundaries = %v, want %v", boundaries, want)
	}
	for i := range want {
		if boundaries[i] != want[i] {
			t.Fatalf("boundaries = %v, want %v", boundaries, want)
		}
	}
}

func TestCuesForChapterAtFiltersForeignChapter(t *testing.T) {
	sec := time.Second
	timeline := mustTimeline(t, 30*sec, []Chapter{
		{ID: "two", Range: Range{Start: 0, End: 15 * sec}},
		{ID: "eight", Range: Range{Start: 15 * sec, End: 30 * sec}},
	}, []Cue{
		{ID: "global", Range: Range{Start: 5 * sec, End: 25 * sec}, Text: "global"},
		{ID: "two-line", ChapterID: "two", Range: Range{Start: 10 * sec, End: 14 * sec}, Text: "two"},
		{ID: "eight-line", ChapterID: "eight", Range: Range{Start: 10 * sec, End: 24 * sec}, Text: "eight"},
	})

	twoCues := timeline.CuesForChapterAt(12*sec, "two")
	if len(twoCues) != 2 || twoCues[0].ID != "global" || twoCues[1].ID != "two-line" {
		t.Fatalf("chapter two cues = %+v", twoCues)
	}

	unselected := timeline.CuesForChapterAt(12*sec, "")
	if len(unselected) != 1 || unselected[0].ID != "global" {
		t.Fatalf("unresolved chapter must only receive global cues, got %+v", unselected)
	}
}

func TestTimelineReturnsDefensiveCopies(t *testing.T) {
	sec := time.Second
	chapters := []Chapter{{ID: "a", Range: Range{Start: 0, End: 10 * sec}}}
	cues := []Cue{{ID: "c", Range: Range{Start: 1 * sec, End: 2 * sec}, Text: "x"}}
	timeline := mustTimeline(t, 10*sec, chapters, cues)

	chapters[0].ID = "mutated"
	cues[0].Text = "mutated"
	gotChapters := timeline.Chapters()
	gotCues := timeline.Cues()
	gotChapters[0].ID = "also-mutated"
	gotCues[0].Text = "also-mutated"

	if chapter, ok := timeline.Chapter("a"); !ok || chapter.ID != "a" {
		t.Fatalf("constructor did not copy chapter input: %+v", chapter)
	}
	if got := timeline.Cues(); got[0].Text != "x" {
		t.Fatalf("accessor leaked mutable cue state: %+v", got)
	}
}

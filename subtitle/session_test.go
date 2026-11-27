package subtitle

import (
	"errors"
	"testing"
	"time"
)

func eightChapterTimeline(t *testing.T) *Timeline {
	t.Helper()
	sec := time.Second
	chapters := make([]Chapter, 0, 8)
	for number := 1; number <= 8; number++ {
		start := time.Duration(number-1) * 10 * sec
		chapters = append(chapters, Chapter{
			ID:    "chapter-" + string(rune('0'+number)),
			Title: "Chapter " + string(rune('0'+number)),
			Range: Range{Start: start, End: start + 10*sec},
		})
	}
	return mustTimeline(t, 80*sec, chapters, []Cue{
		{ID: "chapter-2-open", ChapterID: "chapter-2", Range: Range{Start: 15 * sec, End: 16 * sec}, Text: "第二章的第一句"},
		{ID: "chapter-3-later", ChapterID: "chapter-3", Range: Range{Start: 25 * sec, End: 26 * sec}, Text: "第三章稍后的字幕"},
		{ID: "chapter-8-last", ChapterID: "chapter-8", Range: Range{Start: 75 * sec, End: 76 * sec}, Text: "第八章的最后一句"},
	})
}

func TestBackwardJumpFromEightToTwoDropsOldSubtitle(t *testing.T) {
	sec := time.Second
	session := NewSession(eightChapterTimeline(t))

	chapterEight := session.Seek(75*sec + 500*time.Millisecond)
	if chapterEight.ChapterID() != "chapter-8" {
		t.Fatalf("chapter at 75.5s = %q, want chapter-8", chapterEight.ChapterID())
	}
	if chapterEight.Subtitle != "第八章的最后一句" {
		t.Fatalf("chapter 8 subtitle = %q", chapterEight.Subtitle)
	}

	chapterTwo := session.Seek(15*sec + 500*time.Millisecond)
	if chapterTwo.ChapterID() != "chapter-2" {
		t.Fatalf("chapter after backward jump = %q, want chapter-2", chapterTwo.ChapterID())
	}
	if chapterTwo.Subtitle != "第二章的第一句" {
		t.Fatalf("chapter 2 subtitle = %q", chapterTwo.Subtitle)
	}
	if chapterTwo.Subtitle == chapterEight.Subtitle {
		t.Fatal("old chapter subtitle survived the backward jump")
	}
	if session.IsCurrent(chapterEight) {
		t.Fatal("snapshot from chapter 8 still claims to be current")
	}
	if !session.IsCurrent(chapterTwo) {
		t.Fatal("latest chapter 2 snapshot is not current")
	}

	gap := session.Seek(17 * sec)
	if gap.ChapterID() != "chapter-2" {
		t.Fatalf("chapter in subtitle gap = %q, want chapter-2", gap.ChapterID())
	}
	if gap.HasSubtitle() || gap.Subtitle != "" {
		t.Fatalf("subtitle gap should clear text, got cues=%+v text=%q", gap.Cues, gap.Subtitle)
	}
}

func TestForwardJumpAndExternalClockRebuildSubtitle(t *testing.T) {
	sec := time.Second
	session := NewSession(eightChapterTimeline(t))
	session.Play()

	before := session.Seek(15*sec + 500*time.Millisecond)
	if before.Subtitle != "第二章的第一句" {
		t.Fatalf("initial subtitle = %q", before.Subtitle)
	}
	afterForward := session.Update(75*sec + 500*time.Millisecond)
	if afterForward.ChapterID() != "chapter-8" || afterForward.Subtitle != "第八章的最后一句" {
		t.Fatalf("forward jump snapshot = %+v", afterForward)
	}
	afterBackward := session.Update(25*sec + 500*time.Millisecond)
	if afterBackward.ChapterID() != "chapter-3" || afterBackward.Subtitle != "第三章稍后的字幕" {
		t.Fatalf("backward external-clock jump snapshot = %+v", afterBackward)
	}
	if afterBackward.State != StatePlaying {
		t.Fatalf("Update changed transport state to %q", afterBackward.State)
	}
}

func TestPauseFreezesSubtitleAndResumeRechecksGap(t *testing.T) {
	sec := time.Second
	session := NewSession(eightChapterTimeline(t))

	pausedAtCue := session.Seek(15*sec + 500*time.Millisecond)
	if pausedAtCue.State != StatePaused || pausedAtCue.Subtitle != "第二章的第一句" {
		t.Fatalf("paused snapshot at cue = %+v", pausedAtCue)
	}

	frozen := session.Advance(5 * sec)
	if frozen.Position != pausedAtCue.Position || frozen.Subtitle != pausedAtCue.Subtitle {
		t.Fatalf("paused Advance moved the session: %+v", frozen)
	}

	playing := session.Play()
	if playing.State != StatePlaying || playing.Subtitle != "第二章的第一句" {
		t.Fatalf("Play snapshot = %+v", playing)
	}

	gap := session.Advance(1 * sec)
	if gap.Position != 16*sec+500*time.Millisecond || gap.HasSubtitle() {
		t.Fatalf("advance into subtitle gap = %+v", gap)
	}

	pausedGap := session.Pause()
	if pausedGap.State != StatePaused || pausedGap.HasSubtitle() {
		t.Fatalf("pause in subtitle gap = %+v", pausedGap)
	}
	stillFrozen := session.Advance(-2 * sec)
	if stillFrozen.Position != pausedGap.Position {
		t.Fatalf("paused negative Advance moved from %s to %s", pausedGap.Position, stillFrozen.Position)
	}

	rewound := session.Seek(15*sec + 500*time.Millisecond)
	if rewound.State != StatePaused || rewound.Subtitle != "第二章的第一句" {
		t.Fatalf("seek while paused = %+v", rewound)
	}
}

func TestExactChapterBoundaryShowsNewChapterCue(t *testing.T) {
	sec := time.Second
	timeline := mustTimeline(t, 20*sec, []Chapter{
		{ID: "one", Range: Range{Start: 0, End: 10 * sec}},
		{ID: "two", Range: Range{Start: 10 * sec, End: 20 * sec}},
	}, []Cue{
		{ID: "one-tail", ChapterID: "one", Range: Range{Start: 9 * sec, End: 10 * sec}, Text: "上一章结尾"},
		{ID: "two-head", ChapterID: "two", Range: Range{Start: 10 * sec, End: 11 * sec}, Text: "下一章开头"},
	})
	session := NewSession(timeline)

	before := session.Seek(9*sec + 999*time.Millisecond)
	if before.ChapterID() != "one" || before.Subtitle != "上一章结尾" {
		t.Fatalf("snapshot before boundary = %+v", before)
	}
	atBoundary := session.Seek(10 * sec)
	if atBoundary.ChapterID() != "two" || atBoundary.Subtitle != "下一章开头" {
		t.Fatalf("snapshot at boundary = %+v", atBoundary)
	}
}

func TestAmbiguousCandidatesRequireManualChoice(t *testing.T) {
	sec := time.Second
	timeline := mustTimeline(t, 45*sec, []Chapter{
		{ID: "eight", Title: "Late Eight", Range: Range{Start: 10 * sec, End: 30 * sec}},
		{ID: "two", Title: "Early Two", Range: Range{Start: 20 * sec, End: 40 * sec}},
	}, []Cue{
		{ID: "global", Range: Range{Start: 24 * sec, End: 26 * sec}, Text: "公共字幕"},
		{ID: "eight-cue", ChapterID: "eight", Range: Range{Start: 24 * sec, End: 26 * sec}, Text: "第八章候选字幕"},
		{ID: "two-cue", ChapterID: "two", Range: Range{Start: 24 * sec, End: 26 * sec}, Text: "第二章候选字幕"},
	})
	session := NewSession(timeline)

	ambiguous := session.Seek(25 * sec)
	if ambiguous.Status != ChapterAmbiguous {
		t.Fatalf("status = %q, want ambiguous", ambiguous.Status)
	}
	if ambiguous.Chapter != nil || len(ambiguous.Candidates) != 2 {
		t.Fatalf("ambiguous snapshot must expose candidates without choosing: %+v", ambiguous)
	}
	if got := ambiguous.Subtitle; got != "公共字幕" {
		t.Fatalf("ambiguous chapter should only show global cue, got %q", got)
	}

	selected, err := session.SelectChapter("two")
	if err != nil {
		t.Fatalf("SelectChapter: %v", err)
	}
	if selected.Status != ChapterManual || selected.ChapterID() != "two" {
		t.Fatalf("manual snapshot = %+v", selected)
	}
	if selected.Subtitle != "公共字幕\n第二章候选字幕" {
		t.Fatalf("manual chapter subtitle = %q", selected.Subtitle)
	}

	beforeBadSelect := session.Revision()
	if _, err := session.SelectChapter("missing"); !errors.Is(err, ErrNotCandidate) {
		t.Fatalf("SelectChapter(missing) error = %v, want ErrNotCandidate", err)
	}
	if session.Revision() != beforeBadSelect {
		t.Fatal("failed manual selection changed the session")
	}

	cleared := session.ClearManual()
	if cleared.Status != ChapterAmbiguous || cleared.Chapter != nil {
		t.Fatalf("ClearManual snapshot = %+v", cleared)
	}
}

func TestManualChoiceIsScopedToCurrentCandidates(t *testing.T) {
	sec := time.Second
	timeline := mustTimeline(t, 45*sec, []Chapter{
		{ID: "eight", Range: Range{Start: 10 * sec, End: 30 * sec}},
		{ID: "two", Range: Range{Start: 20 * sec, End: 40 * sec}},
	}, nil)
	session := NewSession(timeline)

	if _, err := session.SelectChapter("two"); !errors.Is(err, ErrNotCandidate) {
		t.Fatalf("selecting at position 0 error = %v, want ErrNotCandidate", err)
	}
	session.Seek(25 * sec)
	if _, err := session.SelectChapter("two"); err != nil {
		t.Fatalf("SelectChapter(two): %v", err)
	}

	// The chosen chapter is still present after advancing, so the human
	// decision survives automatically until it stops being a candidate.
	stillInTwo := session.Seek(35 * sec)
	if stillInTwo.Status != ChapterManual || stillInTwo.ChapterID() != "two" {
		t.Fatalf("manual choice not preserved within chapter: %+v", stillInTwo)
	}

	elsewhere := session.Seek(5 * sec)
	if elsewhere.Status != ChapterNone || elsewhere.Chapter != nil {
		t.Fatalf("manual choice leaked outside its candidates: %+v", elsewhere)
	}
	backToOverlap := session.Seek(25 * sec)
	if backToOverlap.Status != ChapterAmbiguous {
		t.Fatalf("cleared choice should be ambiguous again, got %+v", backToOverlap)
	}
}

func TestSeekByClampsToMediaBounds(t *testing.T) {
	sec := time.Second
	session := NewSession(eightChapterTimeline(t))

	session.Seek(79 * sec)
	atEnd := session.SeekBy(10 * sec)
	if atEnd.Position != 80*sec {
		t.Fatalf("forward clamp position = %s, want 80s", atEnd.Position)
	}
	if atEnd.Chapter != nil || atEnd.HasSubtitle() {
		t.Fatalf("half-open final boundary should have no chapter or cue: %+v", atEnd)
	}
	atStart := session.SeekBy(-100 * sec)
	if atStart.Position != 0 {
		t.Fatalf("backward clamp position = %s, want 0", atStart.Position)
	}
}

func TestTimelineWideCueShowsOutsideChapterGap(t *testing.T) {
	sec := time.Second
	timeline := mustTimeline(t, 30*sec, []Chapter{
		{ID: "one", Range: Range{Start: 0, End: 10 * sec}},
	}, []Cue{
		{ID: "global", Range: Range{Start: 12 * sec, End: 13 * sec}, Text: "章节空隙中的公共字幕"},
	})
	session := NewSession(timeline)

	snap := session.Seek(12*sec + 500*time.Millisecond)
	if snap.Status != ChapterNone || snap.Chapter != nil {
		t.Fatalf("expected no chapter in gap, got %+v", snap)
	}
	if snap.Subtitle != "章节空隙中的公共字幕" {
		t.Fatalf("timeline-wide cue missing in chapter gap: %+v", snap)
	}
	after := session.Seek(14 * sec)
	if after.HasSubtitle() {
		t.Fatalf("timeline-wide cue survived its range: %+v", after)
	}
}

func TestSnapshotCopiesCannotMutateSession(t *testing.T) {
	sec := time.Second
	timeline := mustTimeline(t, 30*sec, []Chapter{
		{ID: "eight", Title: "Eight", Range: Range{Start: 10 * sec, End: 30 * sec}},
		{ID: "two", Title: "Two", Range: Range{Start: 10 * sec, End: 30 * sec}},
	}, nil)
	session := NewSession(timeline)

	snap := session.Seek(15 * sec)
	if _, err := session.SelectChapter("two"); err != nil {
		t.Fatalf("SelectChapter: %v", err)
	}
	snap = session.Snapshot()
	snap.Chapter.Title = "mutated"
	snap.Candidates[0].Title = "mutated too"
	again := session.Snapshot()
	if again.Chapter == nil || again.Chapter.Title != "Two" {
		t.Fatalf("selected chapter changed through snapshot: %+v", again)
	}
	if again.Candidates[0].Title == "mutated too" {
		t.Fatal("returned candidates aliased session state")
	}
}

func TestNilTimelineSessionIsSafe(t *testing.T) {
	session := NewSession(nil)
	snap := session.Seek(time.Second)
	if snap.Status != ChapterNone || snap.HasSubtitle() {
		t.Fatalf("nil timeline snapshot = %+v", snap)
	}
}

package subtitle

import (
	"strings"
	"sync"
	"time"
)

// Snapshot is the complete subtitle-facing state at one media position. A
// snapshot is immutable from the caller's perspective; slices and the chapter
// pointer are defensive copies.
type Snapshot struct {
	Revision   uint64
	Position   time.Duration
	State      PlaybackState
	Status     ChapterStatus
	Chapter    *Chapter
	Candidates []Chapter
	Cues       []Cue
	Subtitle   string
}

// ChapterID returns the resolved or manually selected chapter ID, if any.
func (s Snapshot) ChapterID() string {
	if s.Chapter == nil {
		return ""
	}
	return s.Chapter.ID
}

// HasSubtitle reports whether at least one cue is active, even if its text is
// intentionally empty.
func (s Snapshot) HasSubtitle() bool {
	return len(s.Cues) > 0
}

// Session binds a transport position and playback state to a timeline. Every
// position-changing operation resolves chapters and cues from the new absolute
// position instead of carrying the previous subtitle forward.
type Session struct {
	mu sync.RWMutex

	timeline *Timeline
	position time.Duration
	state    PlaybackState
	manualID string
	revision uint64
	snapshot Snapshot
}

// NewSession creates a paused session at media time zero.
func NewSession(timeline *Timeline) *Session {
	s := &Session{
		timeline: timeline,
		state:    StatePaused,
	}
	s.recomputeLocked(true)
	return s
}

// Snapshot returns the current state.
func (s *Session) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSnapshot(s.snapshot)
}

// Position returns the current media position.
func (s *Session) Position() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.position
}

// State returns the current transport state.
func (s *Session) State() PlaybackState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Revision returns the latest snapshot revision.
func (s *Session) Revision() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision
}

// IsCurrent reports whether snap still belongs to the session's latest
// position/state. It lets asynchronous subtitle work discard stale results
// produced for a chapter the player has already left.
func (s *Session) IsCurrent(snap Snapshot) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return snap.Revision == s.revision
}

// Seek jumps to an absolute position and rebuilds the subtitle state there.
// Negative positions clamp to zero; positions beyond a known duration clamp to
// the end. The pinned chapter choice is kept only if that chapter is still a
// candidate at the new position.
func (s *Session) Seek(at time.Duration) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.position = s.clampLocked(at)
	return s.recomputeLocked(true)
}

// SeekBy jumps by a signed delta. A negative delta is a backward jump.
func (s *Session) SeekBy(delta time.Duration) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.position = s.clampLocked(saturatingAdd(s.position, delta))
	return s.recomputeLocked(true)
}

// Update supplies an absolute position from an external clock. It is useful
// when playback jumps without passing through every intermediate timestamp.
// The transport state is unchanged.
func (s *Session) Update(at time.Duration) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.position = s.clampLocked(at)
	return s.recomputeLocked(true)
}

// Advance moves a playing session by a signed delta. While paused it is a
// no-op, because wall-clock time must not move a frozen subtitle.
func (s *Session) Advance(delta time.Duration) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StatePlaying || delta == 0 {
		return cloneSnapshot(s.snapshot)
	}
	s.position = s.clampLocked(saturatingAdd(s.position, delta))
	return s.recomputeLocked(true)
}

// Play resumes at the current position. The snapshot is rebuilt first, so
// resuming in a subtitle gap cannot resurrect the last displayed line.
func (s *Session) Play() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StatePlaying
	return s.recomputeLocked(true)
}

// Pause freezes the current position and the currently visible subtitle.
func (s *Session) Pause() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StatePaused
	return s.recomputeLocked(true)
}

// SelectChapter records an explicit decision among ambiguous candidates. It
// fails without changing state when id is not a candidate at the current
// position.
func (s *Session) SelectChapter(id string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	candidates := s.timeline.ChaptersAt(s.position)
	for _, chapter := range candidates {
		if chapter.ID == id {
			s.manualID = id
			return s.recomputeLocked(true), nil
		}
	}
	return cloneSnapshot(s.snapshot), ErrNotCandidate
}

// ClearManual drops an explicit chapter choice and returns to automatic
// resolution (which may report ambiguity again).
func (s *Session) ClearManual() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manualID = ""
	return s.recomputeLocked(true)
}

func (s *Session) recomputeLocked(advanceRevision bool) Snapshot {
	if advanceRevision {
		s.revision++
	}
	s.snapshot = s.buildSnapshotLocked(s.timeline.ChaptersAt(s.position))
	return cloneSnapshot(s.snapshot)
}

func (s *Session) buildSnapshotLocked(candidates []Chapter) Snapshot {
	var selected Chapter
	status := ChapterNone

	if len(candidates) == 0 {
		s.manualID = ""
	} else if s.manualID != "" {
		for _, chapter := range candidates {
			if chapter.ID == s.manualID {
				selected = chapter
				status = ChapterManual
				break
			}
		}
		if status != ChapterManual {
			s.manualID = ""
		}
	}

	if status == ChapterNone {
		switch len(candidates) {
		case 0:
			status = ChapterNone
		case 1:
			selected = candidates[0]
			status = ChapterResolved
		default:
			status = ChapterAmbiguous
		}
	}

	var chapter *Chapter
	if status == ChapterResolved || status == ChapterManual {
		copied := selected
		chapter = &copied
	}

	cues := s.timeline.CuesForChapterAt(s.position, selected.ID)
	return Snapshot{
		Revision:   s.revision,
		Position:   s.position,
		State:      s.state,
		Status:     status,
		Chapter:    chapter,
		Candidates: cloneChapters(candidates),
		Cues:       cloneCues(cues),
		Subtitle:   joinSubtitle(cues),
	}
}

func (s *Session) clampLocked(at time.Duration) time.Duration {
	if at < 0 {
		return 0
	}
	if duration := s.timeline.Duration(); duration > 0 && at > duration {
		return duration
	}
	return at
}

func joinSubtitle(cues []Cue) string {
	if len(cues) == 0 {
		return ""
	}
	var text strings.Builder
	for i, cue := range cues {
		if i > 0 {
			text.WriteByte('\n')
		}
		text.WriteString(cue.Text)
	}
	return text.String()
}

func cloneSnapshot(in Snapshot) Snapshot {
	out := in
	if in.Chapter != nil {
		chapter := *in.Chapter
		out.Chapter = &chapter
	}
	out.Candidates = cloneChapters(in.Candidates)
	out.Cues = cloneCues(in.Cues)
	return out
}

const (
	maxDuration = time.Duration(1<<63 - 1)
	minDuration = -maxDuration - 1
)

func saturatingAdd(a, b time.Duration) time.Duration {
	if b > 0 && a > maxDuration-b {
		return maxDuration
	}
	if b < 0 && a < minDuration-b {
		return minDuration
	}
	return a + b
}

// Package subtitle keeps player chapter boundaries and the displayed subtitle
// in sync. It has no dependencies outside the Go standard library.
package subtitle

import (
	"errors"
	"fmt"
	"time"
)

// Errors returned by timeline construction and chapter selection. Callers can
// test them with errors.Is.
var (
	ErrInvalidDuration  = errors.New("subtitle: duration must not be negative")
	ErrMissingID        = errors.New("subtitle: id must not be empty")
	ErrInvalidRange     = errors.New("subtitle: range must satisfy 0 <= start < end")
	ErrOutOfBounds      = errors.New("subtitle: range exceeds media duration")
	ErrDuplicateChapter = errors.New("subtitle: duplicate chapter id")
	ErrDuplicateCue     = errors.New("subtitle: duplicate cue id")
	ErrUnknownChapter   = errors.New("subtitle: cue references an unknown chapter")
	ErrNotCandidate     = errors.New("subtitle: chapter is not a candidate at the current position")
)

// Range is a half-open media interval [Start, End). Half-open ranges make an
// exact chapter boundary belong to the chapter that starts there, not to the
// chapter that just ended.
type Range struct {
	Start time.Duration
	End   time.Duration
}

// Valid reports whether the range is non-negative and non-empty.
func (r Range) Valid() bool {
	return r.Start >= 0 && r.End > r.Start
}

// Contains reports whether at is inside [Start, End).
func (r Range) Contains(at time.Duration) bool {
	return at >= r.Start && at < r.End
}

// String formats the range for diagnostics.
func (r Range) String() string {
	return fmt.Sprintf("[%s,%s)", r.Start, r.End)
}

// Chapter is one chapter boundary. Chapters may overlap; overlap is reported
// as an ambiguous candidate set instead of being resolved by an arbitrary
// tie-break.
type Chapter struct {
	ID    string
	Title string
	Range Range
}

// Cue is one timed subtitle. ChapterID is optional. When set, the cue belongs
// to that chapter and is only shown after that chapter has been resolved, so a
// subtitle from an old chapter cannot survive a jump. An empty ChapterID is a
// timeline-wide cue and is valid even outside a chapter.
type Cue struct {
	ID        string
	ChapterID string
	Range     Range
	Text      string
}

// ChapterStatus describes how the current position maps to chapters.
type ChapterStatus string

const (
	// ChapterNone means no chapter covers the current position.
	ChapterNone ChapterStatus = "none"
	// ChapterResolved means exactly one chapter covers the position.
	ChapterResolved ChapterStatus = "resolved"
	// ChapterAmbiguous means several chapters cover the position and no
	// explicit choice has been made. The player must not pick one silently.
	ChapterAmbiguous ChapterStatus = "ambiguous"
	// ChapterManual means the displayed chapter was selected explicitly from
	// an ambiguous candidate set.
	ChapterManual ChapterStatus = "manual"
)

// PlaybackState is the media transport state. A paused session keeps its
// current snapshot frozen until another command arrives.
type PlaybackState string

const (
	StatePaused  PlaybackState = "paused"
	StatePlaying PlaybackState = "playing"
)

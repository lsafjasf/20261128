package captionsession

import (
	"errors"
	"time"
)

// ErrSegmentFrozen is returned when a revision targets a segment whose
// endPTS is at or below the committed watermark. Such a segment has already
// been presented to the audience and must never be retroactively changed.
var ErrSegmentFrozen = errors.New("captionsession: segment already displayed, revision rejected")

// ErrSegmentNotFound is returned by Replace when the target segment does not
// exist in the provisional (uncommitted) part of the session.
var ErrSegmentNotFound = errors.New("captionsession: segment not found")

// ErrInvalidSegment is returned when a segment has an invalid PTS interval
// or empty identifying key.
var ErrInvalidSegment = errors.New("captionsession: invalid segment")

// ErrResync is reported on a subscriber whose buffer overflowed while it was
// stalled. The client must re-synchronize via Session.Snapshot + SubscribeFrom.
var ErrResync = errors.New("captionsession: subscriber stalled, resync required")

// Segment is one caption line anchored to the media timeline (milliseconds).
// Segments are keyed by ID (e.g. utterance id) and remain mutable until the
// session watermark reaches their EndPTS, at which point they are frozen.
type Segment struct {
	ID       string `json:"id"`
	StartPTS int64  `json:"start_pts"`
	EndPTS   int64  `json:"end_pts"`
	Text     string `json:"text"`
	Final    bool   `json:"final"`
}

// OpKind enumerates the append-only session operations.
type OpKind string

const (
	// OpUpsert inserts a provisional segment or revises an existing one.
	OpUpsert OpKind = "upsert"
	// OpReplace applies a scoped, single-segment text replacement. It only
	// ever touches the named segment, so the surrounding transcript does not
	// reflow or scroll.
	OpReplace OpKind = "replace"
	// OpWatermark advances the committed presentation watermark. Every
	// segment with EndPTS <= watermark becomes immutable.
	OpWatermark OpKind = "watermark"
	// OpSnapshot replaces the prefix of the op log after compaction.
	OpSnapshot OpKind = "snapshot"
)

// Op is one versioned, deterministic state transition.
type Op struct {
	Version   int64     `json:"version"`
	Kind      OpKind    `json:"kind"`
	WallTime  int64     `json:"wall_time_ms"`
	Segment   *Segment  `json:"segment,omitempty"`
	SegmentID string    `json:"segment_id,omitempty"`
	OldText   string    `json:"old_text,omitempty"`
	NewText   string    `json:"new_text,omitempty"`
	Watermark int64     `json:"watermark,omitempty"`
	Snapshot  *Snapshot `json:"snapshot,omitempty"`
}

// Event is delivered to subscribers.
type Event struct {
	Op  Op
	Err error
}

// Snapshot is a compacted materialization of the session at BaseVersion.
type Snapshot struct {
	BaseVersion int64     `json:"base_version"`
	Watermark   int64     `json:"watermark"`
	Segments    []Segment `json:"segments"`
	TakenAt     int64     `json:"taken_at_ms"`
}

// State is the materialized view obtained by replaying a snapshot and a
// sequence of ops. Both live viewers and reconnecting viewers derive the
// exact same State from the same version prefix, so their views converge.
type State struct {
	Version   int64
	Watermark int64
	segments  map[string]Segment
	order     []string
}

// Clock abstracts wall time so that delay compensation is fully testable.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func validSegment(s Segment) bool {
	return s.ID != "" && s.StartPTS >= 0 && s.EndPTS >= s.StartPTS
}

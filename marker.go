package marker

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// Scope identifies the lifetime and resolution domain of a marker.
type Scope string

const (
	// ScopeMedia markers belong to a media asset and use media-local frames.
	ScopeMedia Scope = "media"
	// ScopeClip markers belong to one clip instance in one sequence and use
	// clip-local frames.
	ScopeClip Scope = "clip"
	// ScopeGlobal markers belong to one sequence and use sequence frames.
	ScopeGlobal Scope = "global"
)

var (
	ErrNotFound             = errors.New("marker not found")
	ErrDuplicateID          = errors.New("duplicate marker id")
	ErrInvalidScope         = errors.New("invalid marker scope")
	ErrInvalidMarker        = errors.New("invalid marker")
	ErrInvalidName          = errors.New("invalid marker name")
	ErrInvalidRange         = errors.New("invalid marker range")
	ErrInvalidOwner         = errors.New("invalid marker owner")
	ErrCrossSequence        = errors.New("unresolvable cross-sequence reference")
	ErrMergeConflict        = errors.New("marker merge conflict")
	ErrColorConflict        = errors.New("marker color conflict")
	ErrNameConflict         = errors.New("marker name conflict")
	ErrAliasCycle           = errors.New("marker alias cycle")
	ErrAliasTargetMissing   = errors.New("marker alias target is missing")
	ErrInvalidColor         = errors.New("invalid marker color")
	ErrMarkerOutsideOwner   = errors.New("marker range is outside its owner")
	ErrSequenceMissing      = errors.New("sequence not found")
	ErrMediaMissing         = errors.New("media not found")
	ErrClipMissing          = errors.New("clip not found")
	ErrClipSequenceMismatch = errors.New("clip does not belong to sequence")
)

var colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// FrameRange is a half-open frame interval [Start, End).
type FrameRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// Validate checks that the range is non-empty and representable by the UI.
func (r FrameRange) Validate() error {
	if r.Start < 0 || r.End <= r.Start {
		return fmt.Errorf("%w: must satisfy 0 <= start < end", ErrInvalidRange)
	}
	return nil
}

// Overlaps reports whether two half-open ranges share at least one frame.
func (r FrameRange) Overlaps(other FrameRange) bool {
	return r.Start < other.End && other.Start < r.End
}

// Union returns the smallest range containing both ranges.
func (r FrameRange) Union(other FrameRange) FrameRange {
	return FrameRange{
		Start: min(r.Start, other.Start),
		End:   max(r.End, other.End),
	}
}

// Owner is the stable resolution domain of a marker. ID alone is the marker
// identity; Owner defines where that ID is valid.
type Owner struct {
	Scope      Scope  `json:"scope"`
	MediaID    string `json:"mediaId,omitempty"`
	SequenceID string `json:"sequenceId,omitempty"`
	ClipID     string `json:"clipId,omitempty"`
}

// Validate verifies that exactly the fields required by Scope are present.
func (o Owner) Validate() error {
	switch o.Scope {
	case ScopeMedia:
		if strings.TrimSpace(o.MediaID) == "" || o.SequenceID != "" || o.ClipID != "" {
			return fmt.Errorf("%w: media owner requires only mediaId", ErrInvalidOwner)
		}
	case ScopeClip:
		if o.MediaID != "" || strings.TrimSpace(o.SequenceID) == "" || strings.TrimSpace(o.ClipID) == "" {
			return fmt.Errorf("%w: clip owner requires sequenceId and clipId", ErrInvalidOwner)
		}
	case ScopeGlobal:
		if o.MediaID != "" || strings.TrimSpace(o.SequenceID) == "" || o.ClipID != "" {
			return fmt.Errorf("%w: global owner requires only sequenceId", ErrInvalidOwner)
		}
	default:
		return fmt.Errorf("%w: %q", ErrInvalidScope, o.Scope)
	}
	return nil
}

// Key is the canonical owner key used for equality and indexing.
func (o Owner) Key() string {
	return string(o.Scope) + ":" + o.MediaID + ":" + o.SequenceID + ":" + o.ClipID
}

// ColorChange records a color update without changing marker identity.
type ColorChange struct {
	Revision uint64 `json:"revision"`
	Color    string `json:"color"`
}

// Marker is one independently addressable annotation. Name is deliberately not
// part of identity: five markers with the same name and frame remain five
// markers as long as their IDs are distinct.
type Marker struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Owner        Owner         `json:"owner"`
	Range        FrameRange    `json:"range"`
	Color        string        `json:"color"`
	Revision     uint64        `json:"revision"`
	ColorHistory []ColorChange `json:"colorHistory,omitempty"`
}

func (m Marker) Validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("%w: marker id is empty", ErrInvalidMarker)
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("%w: marker name is empty", ErrInvalidName)
	}
	if err := m.Owner.Validate(); err != nil {
		return err
	}
	if err := m.Range.Validate(); err != nil {
		return err
	}
	if err := validateColor(m.Color); err != nil {
		return err
	}
	if m.Revision == 0 {
		return fmt.Errorf("%w: marker revision must be positive", ErrInvalidMarker)
	}
	if err := m.validateHistory(); err != nil {
		return err
	}
	return nil
}

func (m Marker) validateHistory() error {
	if len(m.ColorHistory) == 0 {
		return nil
	}
	var previous uint64
	for _, change := range m.ColorHistory {
		if change.Revision <= previous {
			return fmt.Errorf("%w: color history revisions must increase", ErrInvalidMarker)
		}
		if err := validateColor(change.Color); err != nil {
			return err
		}
		previous = change.Revision
	}
	if m.ColorHistory[len(m.ColorHistory)-1].Color != m.Color {
		return fmt.Errorf("%w: current color must match last history entry", ErrInvalidMarker)
	}
	if m.ColorHistory[len(m.ColorHistory)-1].Revision > m.Revision {
		return fmt.Errorf("%w: history revision exceeds marker revision", ErrInvalidMarker)
	}
	return nil
}

func validateColor(color string) error {
	if !colorPattern.MatchString(color) {
		return fmt.Errorf("%w: %q must use #RRGGBB", ErrInvalidColor, color)
	}
	return nil
}

// At returns the marker range projected through one clip placement. Media
// markers cannot be projected without a placement; use Project.Resolve instead.
func (m Marker) At(clip Clip) (FrameRange, error) {
	switch m.Owner.Scope {
	case ScopeGlobal:
		return m.Range, nil
	case ScopeClip:
		if clip.ID != m.Owner.ClipID || clip.SequenceID != m.Owner.SequenceID {
			return FrameRange{}, fmt.Errorf("%w: marker %s is not owned by clip %s/%s",
				ErrCrossSequence, m.ID, m.Owner.SequenceID, m.Owner.ClipID)
		}
		start, ok := addInt64(clip.PlacementStart, m.Range.Start)
		if !ok {
			return FrameRange{}, fmt.Errorf("%w: frame overflow", ErrInvalidRange)
		}
		end, ok := addInt64(clip.PlacementStart, m.Range.End)
		if !ok {
			return FrameRange{}, fmt.Errorf("%w: frame overflow", ErrInvalidRange)
		}
		return FrameRange{Start: start, End: end}, nil
	default:
		return FrameRange{}, fmt.Errorf("%w: media marker needs a placement", ErrInvalidScope)
	}
}

func addInt64(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, false
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

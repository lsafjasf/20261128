package marker

import (
	"fmt"
	"sort"
	"strings"
)

// SchemaVersion is bumped only for incompatible JSON shape changes.
const SchemaVersion = 1

// MediaAsset is a source that can be referenced by clips.
type MediaAsset struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Duration int64  `json:"duration"`
}

// Sequence is a timeline that owns global markers and clip instances.
type Sequence struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Duration int64  `json:"duration"`
}

// Clip is one placement of media on a sequence. A clip marker belongs to this
// sequence/clip pair, not to the reusable media asset.
type Clip struct {
	ID             string `json:"id"`
	SequenceID     string `json:"sequenceId"`
	MediaID        string `json:"mediaId"`
	PlacementStart int64  `json:"placementStart"`
	SourceIn       int64  `json:"sourceIn"`
	SourceOut      int64  `json:"sourceOut"`
}

func (c Clip) Key() string {
	return c.SequenceID + "\x00" + c.ID
}

func (c Clip) Duration() int64 {
	return c.SourceOut - c.SourceIn
}

func (c Clip) PlacementEnd() (int64, bool) {
	return addInt64(c.PlacementStart, c.Duration())
}

// Alias preserves an old marker ID after an explicit merge. Alias IDs are
// references, not live markers, and are followed until a canonical marker is
// reached.
type Alias struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Document is the lossless JSON export payload.
type Document struct {
	SchemaVersion int          `json:"schemaVersion"`
	Media         []MediaAsset `json:"media"`
	Sequences     []Sequence   `json:"sequences"`
	Clips         []Clip       `json:"clips"`
	Markers       []Marker     `json:"markers"`
	Aliases       []Alias      `json:"aliases,omitempty"`
}

// Diagnostic identifies one import or reference-resolution problem.
type Diagnostic struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	MarkerID   string `json:"markerId,omitempty"`
	OwnerScope Scope  `json:"ownerScope,omitempty"`
	MediaID    string `json:"mediaId,omitempty"`
	SequenceID string `json:"sequenceId,omitempty"`
	ClipID     string `json:"clipId,omitempty"`
}

func (d Diagnostic) Error() string {
	return d.Code + ": " + d.Message
}

// ValidationError is returned for lossy or unsafe imports. Every independent
// problem is retained so a UI can display all broken references at once.
type ValidationError struct {
	Diagnostics []Diagnostic
}

func (e *ValidationError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "validation failed"
	}
	var b strings.Builder
	b.WriteString("validation failed: ")
	for i, d := range e.Diagnostics {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(d.Error())
	}
	return b.String()
}

func (e *ValidationError) add(code, message string, markerID string, owner Owner) {
	e.Diagnostics = append(e.Diagnostics, Diagnostic{
		Code:       code,
		Message:    message,
		MarkerID:   markerID,
		OwnerScope: owner.Scope,
		MediaID:    owner.MediaID,
		SequenceID: owner.SequenceID,
		ClipID:     owner.ClipID,
	})
}

func (e *ValidationError) errOrNil() error {
	if len(e.Diagnostics) == 0 {
		return nil
	}
	return e
}

func duplicate(value, kind string) string {
	return fmt.Sprintf("duplicate %s %q", kind, value)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

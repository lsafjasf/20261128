// Package project defines the editing-project domain model: an append-only
// operation log, media dependencies and a deterministic state hash.
package project

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// Operation kinds supported by the editing application.
const (
	OpAddClip    = "add_clip"
	OpTrimClip   = "trim_clip"
	OpRemoveClip = "remove_clip"
	OpMoveClip   = "move_clip"
)

// MediaRef describes a media file the project depends on.
type MediaRef struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash"` // sha256 of the media file contents
}

// Clip is a timeline reference to a piece of media.
type Clip struct {
	ID       string  `json:"id"`
	MediaID  string  `json:"media_id"`
	Track    int     `json:"track"`
	Start    float64 `json:"start"`
	End      float64 `json:"end"`
	Position float64 `json:"position"`
}

// Operation is a single, sequentially numbered editing action.
type Operation struct {
	Seq        int       `json:"seq"`
	Kind       string    `json:"kind"`
	ClipID     string    `json:"clip_id,omitempty"`
	MediaID    string    `json:"media_id,omitempty"`
	Track      int       `json:"track,omitempty"`
	Start      float64   `json:"start,omitempty"`
	End        float64   `json:"end,omitempty"`
	Position   float64   `json:"position,omitempty"`
	MediaRef   *MediaRef `json:"media_ref,omitempty"`
	UndoneSeqs []int     `json:"undone_seqs,omitempty"`
}

// Project is the materialized editing state plus its full operation history.
type Project struct {
	ID         string               `json:"id"`
	Version    int                  `json:"version"` // schema version
	Clips      map[string]*Clip     `json:"clips"`
	Media      map[string]*MediaRef `json:"media"`
	Operations []Operation          `json:"operations"`
}

// New creates an empty project.
func New(id string) *Project {
	return &Project{
		ID:      id,
		Version: 1,
		Clips:   make(map[string]*Clip),
		Media:   make(map[string]*MediaRef),
	}
}

// LastSeq returns the highest applied operation sequence number, or 0 when
// no operations have been recorded.
func (p *Project) LastSeq() int {
	if len(p.Operations) == 0 {
		return 0
	}
	return p.Operations[len(p.Operations)-1].Seq
}

// NextSeq returns the sequence number the next operation must use.
func (p *Project) NextSeq() int {
	return p.LastSeq() + 1
}

// Clone returns a deep copy of the project so callers can mutate the copy
// without affecting the receiver.
func (p *Project) Clone() *Project {
	raw, err := json.Marshal(p)
	if err != nil {
		panic(fmt.Sprintf("project clone marshal: %v", err))
	}
	var cp Project
	if err := json.Unmarshal(raw, &cp); err != nil {
		panic(fmt.Sprintf("project clone unmarshal: %v", err))
	}
	return &cp
}

// Apply validates and applies an operation. The operation must carry the
// next expected sequence number; gaps and duplicates are rejected so the
// operation log stays contiguous.
func (p *Project) Apply(op Operation) error {
	if op.Seq != p.NextSeq() {
		return fmt.Errorf("%w: got seq %d, want %d", ErrSeqGap, op.Seq, p.NextSeq())
	}
	if err := p.apply(op); err != nil {
		return err
	}
	p.Operations = append(p.Operations, op)
	return nil
}

var (
	// ErrSeqGap is returned when an operation does not continue the log.
	ErrSeqGap = errors.New("operation sequence gap")
	// ErrUnknownOp is returned for unsupported operation kinds.
	ErrUnknownOp = errors.New("unknown operation kind")
	// ErrClipExists is returned when adding a clip whose ID is in use.
	ErrClipExists = errors.New("clip already exists")
	// ErrClipMissing is returned when an operation targets an unknown clip.
	ErrClipMissing = errors.New("clip not found")
	// ErrMediaMissing is returned when referenced media is not registered.
	ErrMediaMissing = errors.New("media not found")
	// ErrInvalidRange is returned for an inverted trim range.
	ErrInvalidRange = errors.New("invalid trim range")
)

func (p *Project) apply(op Operation) error {
	switch op.Kind {
	case OpAddClip:
		if _, ok := p.Clips[op.ClipID]; ok {
			return fmt.Errorf("%w: %s", ErrClipExists, op.ClipID)
		}
		if _, ok := p.Media[op.MediaID]; !ok {
			return fmt.Errorf("%w: %s", ErrMediaMissing, op.MediaID)
		}
		if op.End <= op.Start {
			return fmt.Errorf("%w: end %v <= start %v", ErrInvalidRange, op.End, op.Start)
		}
		p.Clips[op.ClipID] = &Clip{
			ID:       op.ClipID,
			MediaID:  op.MediaID,
			Track:    op.Track,
			Start:    op.Start,
			End:      op.End,
			Position: op.Position,
		}
	case OpTrimClip:
		clip, ok := p.Clips[op.ClipID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrClipMissing, op.ClipID)
		}
		if op.End <= op.Start {
			return fmt.Errorf("%w: end %v <= start %v", ErrInvalidRange, op.End, op.Start)
		}
		clip.Start, clip.End = op.Start, op.End
	case OpMoveClip:
		clip, ok := p.Clips[op.ClipID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrClipMissing, op.ClipID)
		}
		clip.Track, clip.Position = op.Track, op.Position
	case OpRemoveClip:
		if _, ok := p.Clips[op.ClipID]; !ok {
			return fmt.Errorf("%w: %s", ErrClipMissing, op.ClipID)
		}
		delete(p.Clips, op.ClipID)
	default:
		return fmt.Errorf("%w: %s", ErrUnknownOp, op.Kind)
	}
	return nil
}

// RegisterMedia adds or replaces a media dependency.
func (p *Project) RegisterMedia(ref MediaRef) {
	p.Media[ref.ID] = &ref
}

// StateHash is a stable sha256 over the materialized timeline state
// (clips and media), independent of operation ordering metadata.
func (p *Project) StateHash() string {
	clips := make([]*Clip, 0, len(p.Clips))
	for _, clip := range p.Clips {
		clips = append(clips, clip)
	}
	sortClips(clips)
	media := make([]*MediaRef, 0, len(p.Media))
	for _, ref := range p.Media {
		media = append(media, ref)
	}
	sortMedia(media)
	canonical := struct {
		ID    string      `json:"id"`
		Clips []*Clip     `json:"clips"`
		Media []*MediaRef `json:"media"`
	}{p.ID, clips, media}
	raw, _ := json.Marshal(canonical)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func sortClips(clips []*Clip) {
	for i := 1; i < len(clips); i++ {
		for j := i; j > 0 && clips[j-1].ID > clips[j].ID; j-- {
			clips[j-1], clips[j] = clips[j], clips[j-1]
		}
	}
}

func sortMedia(media []*MediaRef) {
	for i := 1; i < len(media); i++ {
		for j := i; j > 0 && media[j-1].ID > media[j].ID; j-- {
			media[j-1], media[j] = media[j], media[j-1]
		}
	}
}

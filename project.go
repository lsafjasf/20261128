package marker

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// Project is a validated, in-memory marker graph.
type Project struct {
	media     map[string]MediaAsset
	sequences map[string]Sequence
	clips     map[string]Clip
	markers   map[string]Marker
	aliases   map[string]string
}

// NewProject creates an empty project.
func NewProject() *Project {
	return &Project{
		media:     make(map[string]MediaAsset),
		sequences: make(map[string]Sequence),
		clips:     make(map[string]Clip),
		markers:   make(map[string]Marker),
		aliases:   make(map[string]string),
	}
}

// Load validates a document before exposing any of it as a Project. Import is
// atomic: a broken cross-sequence reference cannot partially populate a UI.
func Load(doc Document) (*Project, error) {
	p := NewProject()
	if err := p.replaceWith(doc); err != nil {
		return nil, err
	}
	return p, nil
}

// Import atomically replaces this project with doc.
func (p *Project) Import(doc Document) error {
	next := NewProject()
	if err := next.replaceWith(doc); err != nil {
		return err
	}
	*p = *next
	return nil
}

func (p *Project) replaceWith(doc Document) error {
	media, sequences, clips, markers, aliases, diagnostics := validateDocument(doc)
	if len(diagnostics) > 0 {
		return &ValidationError{Diagnostics: diagnostics}
	}
	p.media = media
	p.sequences = sequences
	p.clips = clips
	p.markers = markers
	p.aliases = aliases
	return nil
}

// Export returns a deterministic, lossless snapshot.
func (p *Project) Export() Document {
	doc := Document{SchemaVersion: SchemaVersion}
	for _, id := range sortedKeys(p.media) {
		doc.Media = append(doc.Media, p.media[id])
	}
	for _, id := range sortedKeys(p.sequences) {
		doc.Sequences = append(doc.Sequences, p.sequences[id])
	}
	clipKeys := sortedKeys(p.clips)
	for _, key := range clipKeys {
		doc.Clips = append(doc.Clips, p.clips[key])
	}
	for _, id := range sortedKeys(p.markers) {
		doc.Markers = append(doc.Markers, cloneMarker(p.markers[id]))
	}
	for _, from := range sortedKeys(p.aliases) {
		doc.Aliases = append(doc.Aliases, Alias{From: from, To: p.aliases[from]})
	}
	return doc
}

// AddMedia registers a source asset.
func (p *Project) AddMedia(asset MediaAsset) error {
	if strings.TrimSpace(asset.ID) == "" || strings.TrimSpace(asset.Name) == "" || asset.Duration <= 0 {
		return fmt.Errorf("%w: media requires id, name, and positive duration", ErrMediaMissing)
	}
	if _, exists := p.media[asset.ID]; exists {
		return fmt.Errorf("%w: media %q", ErrDuplicateID, asset.ID)
	}
	p.media[asset.ID] = asset
	return nil
}

// AddSequence registers a timeline.
func (p *Project) AddSequence(sequence Sequence) error {
	if strings.TrimSpace(sequence.ID) == "" || strings.TrimSpace(sequence.Name) == "" || sequence.Duration <= 0 {
		return fmt.Errorf("%w: sequence requires id, name, and positive duration", ErrSequenceMissing)
	}
	if _, exists := p.sequences[sequence.ID]; exists {
		return fmt.Errorf("%w: sequence %q", ErrDuplicateID, sequence.ID)
	}
	p.sequences[sequence.ID] = sequence
	return nil
}

// AddClip registers a clip placement after checking ownership and bounds.
func (p *Project) AddClip(clip Clip) error {
	if strings.TrimSpace(clip.ID) == "" || strings.TrimSpace(clip.SequenceID) == "" || strings.TrimSpace(clip.MediaID) == "" {
		return fmt.Errorf("%w: clip requires id, sequenceId, and mediaId", ErrClipMissing)
	}
	if _, exists := p.clips[clip.Key()]; exists {
		return fmt.Errorf("%w: clip %s/%s", ErrDuplicateID, clip.SequenceID, clip.ID)
	}
	if diagnostics := p.validateClip(clip); len(diagnostics) > 0 {
		return &ValidationError{Diagnostics: diagnostics}
	}
	p.clips[clip.Key()] = clip
	return nil
}

// AddMarker inserts a marker. An empty ID is assigned a new stable ID. Existing
// IDs are never regenerated because callers may already hold references.
func (p *Project) AddMarker(input Marker) (Marker, error) {
	if input.ID == "" {
		id, err := newMarkerID(input.Owner.Scope)
		if err != nil {
			return Marker{}, err
		}
		input.ID = id
	}
	if _, exists := p.markers[input.ID]; exists {
		return Marker{}, fmt.Errorf("%w: marker %q", ErrDuplicateID, input.ID)
	}
	if _, exists := p.aliases[input.ID]; exists {
		return Marker{}, fmt.Errorf("%w: marker id %q is a retired alias", ErrDuplicateID, input.ID)
	}
	if input.Revision == 0 {
		input.Revision = 1
	}
	input.ColorHistory = append([]ColorChange(nil), input.ColorHistory...)
	if len(input.ColorHistory) == 0 {
		input.ColorHistory = []ColorChange{{Revision: input.Revision, Color: input.Color}}
	} else if input.ColorHistory[len(input.ColorHistory)-1].Revision != input.Revision {
		input.ColorHistory = append(input.ColorHistory, ColorChange{Revision: input.Revision, Color: input.Color})
	}
	if err := input.Validate(); err != nil {
		return Marker{}, err
	}
	diagnostics := p.validateMarker(input, false)
	if len(diagnostics) > 0 {
		return Marker{}, &ValidationError{Diagnostics: diagnostics}
	}
	p.markers[input.ID] = cloneMarker(input)
	return cloneMarker(input), nil
}

func newMarkerID(scope Scope) (string, error) {
	if scope != ScopeMedia && scope != ScopeClip && scope != ScopeGlobal {
		return "", fmt.Errorf("%w: %q", ErrInvalidScope, scope)
	}
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate marker id: %w", err)
	}
	return "m_" + string(scope) + "_" + hex.EncodeToString(raw[:]), nil
}

func cloneMarker(m Marker) Marker {
	m.ColorHistory = append([]ColorChange(nil), m.ColorHistory...)
	return m
}

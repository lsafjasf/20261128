package marker

import (
	"fmt"
	"sort"
	"strings"
)

// MarkerInstance is a marker projected onto a concrete sequence frame range.
// The same media marker may produce several instances when its media is used by
// several clips.
type MarkerInstance struct {
	Marker   Marker
	Sequence FrameRange
	ClipID   string
}

// ResolveID follows merge aliases to the live marker. References are always
// resolved through this method; a merged-away ID never becomes ambiguous.
func (p *Project) ResolveID(id string) (string, error) {
	seen := make(map[string]bool)
	current := id
	for {
		if _, ok := p.markers[current]; ok {
			return current, nil
		}
		next, ok := p.aliases[current]
		if !ok {
			return "", fmt.Errorf("%w: %q", ErrNotFound, id)
		}
		if seen[current] {
			return "", fmt.Errorf("%w: %q", ErrAliasCycle, id)
		}
		seen[current] = true
		current = next
	}
}

// Get returns the live marker referenced by id, following aliases.
func (p *Project) Get(id string) (Marker, error) {
	resolved, err := p.ResolveID(id)
	if err != nil {
		return Marker{}, err
	}
	return cloneMarker(p.markers[resolved]), nil
}

// OwnerMarkers returns all live markers with exactly this owner, ordered by
// marker ID. Equal names and ranges are intentionally retained.
func (p *Project) OwnerMarkers(owner Owner) ([]Marker, error) {
	if err := owner.Validate(); err != nil {
		return nil, err
	}
	if len(p.validateMarker(Marker{
		ID: "probe", Name: "probe", Owner: owner,
		Range: FrameRange{Start: 0, End: 1}, Color: "#000000", Revision: 1,
	}, false)) > 0 {
		return nil, fmt.Errorf("%w: owner does not resolve", ErrInvalidOwner)
	}
	var result []Marker
	for _, candidate := range p.markers {
		if candidate.Owner.Key() == owner.Key() {
			result = append(result, cloneMarker(candidate))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// Overlapping returns markers with the same owner whose ranges intersect r.
func (p *Project) Overlapping(owner Owner, r FrameRange) ([]Marker, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	markers, err := p.OwnerMarkers(owner)
	if err != nil {
		return nil, err
	}
	result := markers[:0]
	for _, candidate := range markers {
		if candidate.Range.Overlaps(r) {
			result = append(result, candidate)
		}
	}
	return result, nil
}

// At returns every marker visible on sequenceID at frame, including media
// markers projected through every matching clip placement.
func (p *Project) At(sequenceID string, frame int64) ([]MarkerInstance, error) {
	sequence, ok := p.sequences[sequenceID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrSequenceMissing, sequenceID)
	}
	if frame < 0 || frame >= sequence.Duration {
		return nil, fmt.Errorf("%w: frame %d is outside sequence %q", ErrInvalidRange, frame, sequenceID)
	}

	var result []MarkerInstance
	for _, candidate := range p.markers {
		result = append(result, p.instancesAt(candidate, sequenceID, frame)...)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Marker.ID != result[j].Marker.ID {
			return result[i].Marker.ID < result[j].Marker.ID
		}
		return result[i].ClipID < result[j].ClipID
	})
	return result, nil
}

func (p *Project) instancesAt(candidate Marker, sequenceID string, frame int64) []MarkerInstance {
	switch candidate.Owner.Scope {
	case ScopeGlobal:
		if candidate.Owner.SequenceID != sequenceID {
			return nil
		}
		if contains(candidate.Range, frame) {
			return []MarkerInstance{{Marker: cloneMarker(candidate), Sequence: candidate.Range}}
		}
	case ScopeClip:
		if candidate.Owner.SequenceID != sequenceID {
			return nil
		}
		clip, ok := p.clips[sequenceID+"\x00"+candidate.Owner.ClipID]
		if !ok || frame < clip.PlacementStart {
			return nil
		}
		local := frame - clip.PlacementStart
		if local >= clip.Duration() || !contains(candidate.Range, local) {
			return nil
		}
		start, _ := addInt64(clip.PlacementStart, candidate.Range.Start)
		end, _ := addInt64(clip.PlacementStart, candidate.Range.End)
		return []MarkerInstance{{
			Marker: cloneMarker(candidate), Sequence: FrameRange{Start: start, End: end}, ClipID: clip.ID,
		}}
	case ScopeMedia:
		var result []MarkerInstance
		for _, clip := range p.clips {
			if clip.SequenceID != sequenceID || clip.MediaID != candidate.Owner.MediaID {
				continue
			}
			if frame < clip.PlacementStart || frame >= clip.PlacementStart+clip.Duration() {
				continue
			}
			sourceFrame := clip.SourceIn + (frame - clip.PlacementStart)
			if !contains(candidate.Range, sourceFrame) {
				continue
			}
			visibleStart := max(candidate.Range.Start, clip.SourceIn)
			visibleEnd := min(candidate.Range.End, clip.SourceOut)
			start := clip.PlacementStart + (visibleStart - clip.SourceIn)
			end := clip.PlacementStart + (visibleEnd - clip.SourceIn)
			result = append(result, MarkerInstance{
				Marker: cloneMarker(candidate), Sequence: FrameRange{Start: start, End: end}, ClipID: clip.ID,
			})
		}
		return result
	}
	return nil
}

func contains(r FrameRange, frame int64) bool {
	return frame >= r.Start && frame < r.End
}

// Reference names a marker from a surrounding context. A non-empty SequenceID
// asserts that the reference must be usable on that sequence.
type Reference struct {
	MarkerID   string `json:"markerId"`
	SequenceID string `json:"sequenceId,omitempty"`
}

// Resolve projects a marker reference into a sequence context. Clip and global
// markers are sequence-local. Media markers are portable, but still require a
// visible clip using the media when a sequence context is supplied.
func (p *Project) Resolve(ref Reference) ([]MarkerInstance, error) {
	candidate, err := p.Get(strings.TrimSpace(ref.MarkerID))
	if err != nil {
		return nil, err
	}
	if ref.SequenceID == "" {
		if candidate.Owner.Scope == ScopeMedia {
			return []MarkerInstance{{Marker: candidate, Sequence: candidate.Range}}, nil
		}
		return nil, fmt.Errorf("%w: marker %s needs sequence %q",
			ErrCrossSequence, candidate.ID, candidate.Owner.SequenceID)
	}
	if _, ok := p.sequences[ref.SequenceID]; !ok {
		return nil, fmt.Errorf("%w: marker %s references missing sequence %q",
			ErrCrossSequence, candidate.ID, ref.SequenceID)
	}

	switch candidate.Owner.Scope {
	case ScopeGlobal, ScopeClip:
		if candidate.Owner.SequenceID != ref.SequenceID {
			return nil, fmt.Errorf("%w: %s marker %s belongs to sequence %q and cannot be used in sequence %q",
				ErrCrossSequence, candidate.Owner.Scope, candidate.ID,
				candidate.Owner.SequenceID, ref.SequenceID)
		}
	case ScopeMedia:
		var result []MarkerInstance
		for _, clip := range p.clips {
			if clip.SequenceID != ref.SequenceID || clip.MediaID != candidate.Owner.MediaID {
				continue
			}
			start := max(candidate.Range.Start, clip.SourceIn)
			end := min(candidate.Range.End, clip.SourceOut)
			if end <= start {
				continue
			}
			result = append(result, MarkerInstance{
				Marker: cloneMarker(candidate),
				Sequence: FrameRange{
					Start: clip.PlacementStart + start - clip.SourceIn,
					End:   clip.PlacementStart + end - clip.SourceIn,
				},
				ClipID: clip.ID,
			})
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("%w: media marker %s has no visible placement in sequence %q",
				ErrCrossSequence, candidate.ID, ref.SequenceID)
		}
		sort.Slice(result, func(i, j int) bool { return result[i].ClipID < result[j].ClipID })
		return result, nil
	}

	return p.projectWhole(candidate, ref.SequenceID)
}

func (p *Project) projectWhole(candidate Marker, sequenceID string) ([]MarkerInstance, error) {
	switch candidate.Owner.Scope {
	case ScopeGlobal:
		return []MarkerInstance{{Marker: candidate, Sequence: candidate.Range}}, nil
	case ScopeClip:
		clip, ok := p.clips[sequenceID+"\x00"+candidate.Owner.ClipID]
		if !ok {
			return nil, fmt.Errorf("%w: clip %s/%s is missing",
				ErrCrossSequence, sequenceID, candidate.Owner.ClipID)
		}
		return []MarkerInstance{{
			Marker: candidate,
			Sequence: FrameRange{
				Start: clip.PlacementStart + candidate.Range.Start,
				End:   clip.PlacementStart + candidate.Range.End,
			},
			ClipID: clip.ID,
		}}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported scope %q", ErrInvalidScope, candidate.Owner.Scope)
	}
}

package marker

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// SetColor changes only the marker's color. ID, owner, range, and all
// references remain untouched; a revisioned history entry is appended.
func (p *Project) SetColor(id, color string) (Marker, error) {
	color = strings.ToUpper(color)
	if err := validateColor(color); err != nil {
		return Marker{}, err
	}
	resolved, err := p.ResolveID(id)
	if err != nil {
		return Marker{}, err
	}
	candidate := p.markers[resolved]
	if candidate.Color == color {
		return cloneMarker(candidate), nil
	}
	if candidate.Revision == math.MaxUint64 {
		return Marker{}, fmt.Errorf("%w: marker revision overflow", ErrInvalidRange)
	}
	if len(candidate.ColorHistory) == 0 {
		candidate.ColorHistory = append(candidate.ColorHistory, ColorChange{
			Revision: candidate.Revision,
			Color:    candidate.Color,
		})
	}
	candidate.Revision++
	candidate.Color = color
	candidate.ColorHistory = append(candidate.ColorHistory, ColorChange{
		Revision: candidate.Revision,
		Color:    color,
	})
	p.markers[resolved] = candidate
	return cloneMarker(candidate), nil
}

// MergeOptions controls the visible attributes of a merged marker. Name and
// Color are required only when the source markers disagree.
type MergeOptions struct {
	Name           string
	Color          string
	RequireOverlap bool
}

// Merge explicitly combines two or more markers with the same owner. The
// smallest source ID is retained as canonical, every other source ID becomes an
// alias, and the merged range is the union of the inputs.
func (p *Project) Merge(ids []string, options MergeOptions) (Marker, error) {
	unique := make(map[string]bool)
	var canonicalIDs []string
	for _, id := range ids {
		resolved, err := p.ResolveID(strings.TrimSpace(id))
		if err != nil {
			return Marker{}, err
		}
		if !unique[resolved] {
			unique[resolved] = true
			canonicalIDs = append(canonicalIDs, resolved)
		}
	}
	if len(canonicalIDs) < 2 {
		return Marker{}, fmt.Errorf("%w: merge needs at least two distinct live markers", ErrMergeConflict)
	}
	sort.Strings(canonicalIDs)

	sources := make([]Marker, 0, len(canonicalIDs))
	for _, id := range canonicalIDs {
		sources = append(sources, cloneMarker(p.markers[id]))
	}
	owner := sources[0].Owner
	mergedRange := sources[0].Range
	for _, candidate := range sources[1:] {
		if candidate.Owner.Key() != owner.Key() {
			return Marker{}, fmt.Errorf(
				"%w: cannot merge owners %s and %s; markers must remain in one resolution domain",
				ErrMergeConflict, owner.Key(), candidate.Owner.Key(),
			)
		}
		mergedRange = mergedRange.Union(candidate.Range)
	}
	if options.RequireOverlap {
		if err := requireConnected(sources); err != nil {
			return Marker{}, err
		}
	}

	name, err := mergeName(sources, options.Name)
	if err != nil {
		return Marker{}, err
	}
	color, err := mergeColor(sources, options.Color)
	if err != nil {
		return Marker{}, err
	}
	history, lastRevision := mergeHistory(sources, color)
	if lastRevision == math.MaxUint64 {
		return Marker{}, fmt.Errorf("%w: marker revision overflow", ErrInvalidRange)
	}
	merged := Marker{
		ID:           canonicalIDs[0],
		Name:         name,
		Owner:        owner,
		Range:        mergedRange,
		Color:        color,
		Revision:     lastRevision + 1,
		ColorHistory: history,
	}
	if err := merged.Validate(); err != nil {
		return Marker{}, err
	}
	if diagnostics := p.validateMarker(merged, false); len(diagnostics) > 0 {
		return Marker{}, &ValidationError{Diagnostics: diagnostics}
	}

	for _, oldID := range canonicalIDs[1:] {
		delete(p.markers, oldID)
		p.aliases[oldID] = merged.ID
	}
	p.markers[merged.ID] = cloneMarker(merged)
	return cloneMarker(merged), nil
}

func requireConnected(markers []Marker) error {
	ordered := append([]Marker(nil), markers...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Range.Start != ordered[j].Range.Start {
			return ordered[i].Range.Start < ordered[j].Range.Start
		}
		return ordered[i].Range.End < ordered[j].Range.End
	})
	reach := ordered[0].Range.End
	for _, candidate := range ordered[1:] {
		if candidate.Range.Start >= reach {
			return fmt.Errorf(
				"%w: ranges are not connected; %s ends before %s starts",
				ErrMergeConflict, ordered[0].ID, candidate.ID,
			)
		}
		reach = max(reach, candidate.Range.End)
	}
	return nil
}

func mergeName(markers []Marker, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested, nil
	}
	common := markers[0].Name
	for _, candidate := range markers[1:] {
		if candidate.Name != common {
			return "", fmt.Errorf("%w: source names differ; provide MergeOptions.Name", ErrNameConflict)
		}
	}
	return common, nil
}

func mergeColor(markers []Marker, requested string) (string, error) {
	requested = strings.ToUpper(strings.TrimSpace(requested))
	if requested != "" {
		if err := validateColor(requested); err != nil {
			return "", err
		}
		return requested, nil
	}
	common := strings.ToUpper(markers[0].Color)
	for _, candidate := range markers[1:] {
		if strings.ToUpper(candidate.Color) != common {
			return "", fmt.Errorf("%w: source colors differ; provide MergeOptions.Color", ErrColorConflict)
		}
	}
	return common, nil
}

type historyEntry struct {
	markerID string
	change   ColorChange
}

func mergeHistory(markers []Marker, finalColor string) ([]ColorChange, uint64) {
	var entries []historyEntry
	for _, candidate := range markers {
		history := candidate.ColorHistory
		if len(history) == 0 {
			history = []ColorChange{{Revision: candidate.Revision, Color: candidate.Color}}
		}
		for _, change := range history {
			entries = append(entries, historyEntry{markerID: candidate.ID, change: change})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].change.Revision != entries[j].change.Revision {
			return entries[i].change.Revision < entries[j].change.Revision
		}
		if entries[i].markerID != entries[j].markerID {
			return entries[i].markerID < entries[j].markerID
		}
		return entries[i].change.Color < entries[j].change.Color
	})

	var history []ColorChange
	var revision uint64
	for _, entry := range entries {
		if entry.change.Revision > revision {
			revision = entry.change.Revision
		} else {
			if revision == math.MaxUint64 {
				return nil, math.MaxUint64
			}
			revision++
		}
		if len(history) == 0 || history[len(history)-1].Color != entry.change.Color {
			history = append(history, ColorChange{Revision: revision, Color: entry.change.Color})
		}
	}
	if len(history) == 0 || history[len(history)-1].Color != finalColor {
		if revision == math.MaxUint64 {
			return nil, math.MaxUint64
		}
		revision++
		history = append(history, ColorChange{Revision: revision, Color: finalColor})
	}
	return history, revision
}

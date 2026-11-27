package marker

import (
	"errors"
	"fmt"
	"strings"
)

func problem(code, message string, owner Owner) Diagnostic {
	return Diagnostic{
		Code:       code,
		Message:    message,
		OwnerScope: owner.Scope,
		MediaID:    owner.MediaID,
		SequenceID: owner.SequenceID,
		ClipID:     owner.ClipID,
	}
}

func markerProblem(code, message, markerID string, owner Owner) Diagnostic {
	diagnostic := problem(code, message, owner)
	diagnostic.MarkerID = markerID
	return diagnostic
}

func validateDocument(doc Document) (
	map[string]MediaAsset,
	map[string]Sequence,
	map[string]Clip,
	map[string]Marker,
	map[string]string,
	[]Diagnostic,
) {
	media := make(map[string]MediaAsset, len(doc.Media))
	sequences := make(map[string]Sequence, len(doc.Sequences))
	clips := make(map[string]Clip, len(doc.Clips))
	markers := make(map[string]Marker, len(doc.Markers))
	aliases := make(map[string]string, len(doc.Aliases))
	var diagnostics []Diagnostic

	if doc.SchemaVersion != SchemaVersion {
		diagnostics = append(diagnostics, problem(
			"unsupported_schema",
			fmt.Sprintf("schemaVersion %d is not supported; expected %d", doc.SchemaVersion, SchemaVersion),
			Owner{},
		))
	}

	for _, asset := range doc.Media {
		if strings.TrimSpace(asset.ID) == "" || strings.TrimSpace(asset.Name) == "" || asset.Duration <= 0 {
			diagnostics = append(diagnostics, problem(
				"invalid_media", "media requires id, name, and positive duration", Owner{Scope: ScopeMedia, MediaID: asset.ID},
			))
			continue
		}
		if _, exists := media[asset.ID]; exists {
			diagnostics = append(diagnostics, problem(
				"duplicate_media", duplicate(asset.ID, "media"), Owner{Scope: ScopeMedia, MediaID: asset.ID},
			))
			continue
		}
		media[asset.ID] = asset
	}

	for _, sequence := range doc.Sequences {
		if strings.TrimSpace(sequence.ID) == "" || strings.TrimSpace(sequence.Name) == "" || sequence.Duration <= 0 {
			diagnostics = append(diagnostics, problem(
				"invalid_sequence", "sequence requires id, name, and positive duration",
				Owner{Scope: ScopeGlobal, SequenceID: sequence.ID},
			))
			continue
		}
		if _, exists := sequences[sequence.ID]; exists {
			diagnostics = append(diagnostics, problem(
				"duplicate_sequence", duplicate(sequence.ID, "sequence"),
				Owner{Scope: ScopeGlobal, SequenceID: sequence.ID},
			))
			continue
		}
		sequences[sequence.ID] = sequence
	}

	for _, clip := range doc.Clips {
		clipDiagnostics := validateClipAgainst(clip, media, sequences)
		if len(clipDiagnostics) > 0 {
			diagnostics = append(diagnostics, clipDiagnostics...)
			continue
		}
		if _, exists := clips[clip.Key()]; exists {
			diagnostics = append(diagnostics, problem(
				"duplicate_clip",
				fmt.Sprintf("duplicate clip %s/%s", clip.SequenceID, clip.ID),
				Owner{Scope: ScopeClip, SequenceID: clip.SequenceID, ClipID: clip.ID},
			))
			continue
		}
		clips[clip.Key()] = clip
	}

	for _, candidate := range doc.Markers {
		markerDiagnostics := validateMarkerAgainst(candidate, media, sequences, clips)
		if len(markerDiagnostics) > 0 {
			diagnostics = append(diagnostics, markerDiagnostics...)
			continue
		}
		if _, exists := markers[candidate.ID]; exists {
			diagnostics = append(diagnostics, markerProblem(
				"duplicate_marker_id", duplicate(candidate.ID, "marker id"), candidate.ID, candidate.Owner,
			))
			continue
		}
		markers[candidate.ID] = cloneMarker(candidate)
	}

	for _, alias := range doc.Aliases {
		if strings.TrimSpace(alias.From) == "" || strings.TrimSpace(alias.To) == "" || alias.From == alias.To {
			diagnostics = append(diagnostics, problem(
				"invalid_alias", "alias requires distinct non-empty ids", Owner{},
			))
			continue
		}
		if _, exists := aliases[alias.From]; exists {
			diagnostics = append(diagnostics, problem(
				"duplicate_alias", duplicate(alias.From, "alias source"), Owner{},
			))
			continue
		}
		if _, isLive := markers[alias.From]; isLive {
			diagnostics = append(diagnostics, problem(
				"alias_id_collision",
				fmt.Sprintf("alias source %q is also a live marker; references would be ambiguous", alias.From),
				Owner{},
			))
			continue
		}
		aliases[alias.From] = alias.To
	}

	diagnostics = append(diagnostics, validateAliases(aliases, markers)...)
	return media, sequences, clips, markers, aliases, diagnostics
}

func validateClipAgainst(
	clip Clip,
	media map[string]MediaAsset,
	sequences map[string]Sequence,
) []Diagnostic {
	owner := Owner{Scope: ScopeClip, SequenceID: clip.SequenceID, ClipID: clip.ID}
	var diagnostics []Diagnostic
	if strings.TrimSpace(clip.ID) == "" || strings.TrimSpace(clip.SequenceID) == "" || strings.TrimSpace(clip.MediaID) == "" {
		diagnostics = append(diagnostics, problem(
			"invalid_clip", "clip requires id, sequenceId, and mediaId", owner,
		))
		return diagnostics
	}
	sequence, hasSequence := sequences[clip.SequenceID]
	if !hasSequence {
		diagnostics = append(diagnostics, problem(
			"missing_sequence",
			fmt.Sprintf("clip %s/%s references missing sequence %q", clip.SequenceID, clip.ID, clip.SequenceID),
			owner,
		))
	}
	asset, hasMedia := media[clip.MediaID]
	if !hasMedia {
		diagnostics = append(diagnostics, problem(
			"missing_media",
			fmt.Sprintf("clip %s/%s references missing media %q", clip.SequenceID, clip.ID, clip.MediaID),
			owner,
		))
	}
	if clip.PlacementStart < 0 {
		diagnostics = append(diagnostics, problem("invalid_clip", "clip placementStart must be non-negative", owner))
	}
	if clip.SourceIn < 0 || clip.SourceOut <= clip.SourceIn {
		diagnostics = append(diagnostics, problem(
			"invalid_clip", "clip must satisfy 0 <= sourceIn < sourceOut", owner,
		))
	} else if hasMedia && clip.SourceOut > asset.Duration {
		diagnostics = append(diagnostics, problem(
			"invalid_clip",
			fmt.Sprintf("clip source range [%d,%d) exceeds media duration %d", clip.SourceIn, clip.SourceOut, asset.Duration),
			owner,
		))
	}
	if end, ok := clip.PlacementEnd(); !ok || clip.PlacementStart < 0 {
		diagnostics = append(diagnostics, problem("invalid_clip", "clip placement overflows frame space", owner))
	} else if hasSequence && end > sequence.Duration {
		diagnostics = append(diagnostics, problem(
			"invalid_clip",
			fmt.Sprintf("clip placement [%d,%d) exceeds sequence duration %d", clip.PlacementStart, end, sequence.Duration),
			owner,
		))
	}
	return diagnostics
}

func validateMarkerAgainst(
	candidate Marker,
	media map[string]MediaAsset,
	sequences map[string]Sequence,
	clips map[string]Clip,
) []Diagnostic {
	var diagnostics []Diagnostic
	if err := candidate.Validate(); err != nil {
		code := "invalid_marker"
		switch {
		case errors.Is(err, ErrInvalidOwner), errors.Is(err, ErrInvalidScope):
			code = "invalid_owner"
		case errors.Is(err, ErrInvalidName):
			code = "invalid_name"
		case errors.Is(err, ErrInvalidRange):
			code = "invalid_range"
		case errors.Is(err, ErrInvalidColor):
			code = "invalid_color"
		}
		diagnostics = append(diagnostics, markerProblem(code, err.Error(), candidate.ID, candidate.Owner))
		return diagnostics
	}

	switch candidate.Owner.Scope {
	case ScopeMedia:
		asset, exists := media[candidate.Owner.MediaID]
		if !exists {
			diagnostics = append(diagnostics, markerProblem(
				"missing_media",
				fmt.Sprintf("media marker %s references missing media %q", candidate.ID, candidate.Owner.MediaID),
				candidate.ID,
				candidate.Owner,
			))
		} else if candidate.Range.End > asset.Duration {
			diagnostics = append(diagnostics, markerProblem(
				"marker_outside_owner",
				fmt.Sprintf("media marker %s range [%d,%d) exceeds media duration %d",
					candidate.ID, candidate.Range.Start, candidate.Range.End, asset.Duration),
				candidate.ID,
				candidate.Owner,
			))
		}
	case ScopeGlobal:
		sequence, exists := sequences[candidate.Owner.SequenceID]
		if !exists {
			diagnostics = append(diagnostics, markerProblem(
				"missing_sequence",
				fmt.Sprintf("global marker %s references missing sequence %q", candidate.ID, candidate.Owner.SequenceID),
				candidate.ID,
				candidate.Owner,
			))
		} else if candidate.Range.End > sequence.Duration {
			diagnostics = append(diagnostics, markerProblem(
				"marker_outside_owner",
				fmt.Sprintf("global marker %s range [%d,%d) exceeds sequence duration %d",
					candidate.ID, candidate.Range.Start, candidate.Range.End, sequence.Duration),
				candidate.ID,
				candidate.Owner,
			))
		}
	case ScopeClip:
		if _, exists := sequences[candidate.Owner.SequenceID]; !exists {
			diagnostics = append(diagnostics, markerProblem(
				"missing_sequence",
				fmt.Sprintf("clip marker %s references missing sequence %q", candidate.ID, candidate.Owner.SequenceID),
				candidate.ID,
				candidate.Owner,
			))
			return diagnostics
		}
		clip, exists := clips[candidate.Owner.SequenceID+"\x00"+candidate.Owner.ClipID]
		if !exists {
			if other := findClipSequence(clips, candidate.Owner.ClipID); other != "" {
				diagnostics = append(diagnostics, markerProblem(
					"cross_sequence_reference",
					fmt.Sprintf("clip marker %s targets clip %q in sequence %q, but the clip belongs to sequence %q; clip markers cannot leave their sequence",
						candidate.ID, candidate.Owner.ClipID, candidate.Owner.SequenceID, other),
					candidate.ID,
					candidate.Owner,
				))
			} else {
				diagnostics = append(diagnostics, markerProblem(
					"missing_clip",
					fmt.Sprintf("clip marker %s references missing clip %s/%s",
						candidate.ID, candidate.Owner.SequenceID, candidate.Owner.ClipID),
					candidate.ID,
					candidate.Owner,
				))
			}
		} else if candidate.Range.End > clip.Duration() {
			diagnostics = append(diagnostics, markerProblem(
				"marker_outside_owner",
				fmt.Sprintf("clip marker %s range [%d,%d) exceeds clip duration %d",
					candidate.ID, candidate.Range.Start, candidate.Range.End, clip.Duration()),
				candidate.ID,
				candidate.Owner,
			))
		}
	}
	return diagnostics
}

func findClipSequence(clips map[string]Clip, clipID string) string {
	for _, key := range sortedKeys(clips) {
		clip := clips[key]
		if clip.ID == clipID {
			return clip.SequenceID
		}
	}
	return ""
}

func validateAliases(aliases map[string]string, markers map[string]Marker) []Diagnostic {
	var diagnostics []Diagnostic
	for _, from := range sortedKeys(aliases) {
		seen := map[string]bool{from: true}
		current := aliases[from]
		for {
			if _, isMarker := markers[current]; isMarker {
				break
			}
			next, isAlias := aliases[current]
			if !isAlias {
				diagnostics = append(diagnostics, problem(
					"alias_target_missing",
					fmt.Sprintf("alias %q points to %q, which is neither a live marker nor another alias", from, current),
					Owner{},
				))
				break
			}
			if seen[current] {
				diagnostics = append(diagnostics, problem(
					"alias_cycle",
					fmt.Sprintf("alias chain starting at %q contains a cycle at %q", from, current),
					Owner{},
				))
				break
			}
			seen[current] = true
			current = next
		}
	}
	return diagnostics
}

package marker_test

import (
	"errors"
	"testing"

	marker "github.com/lsafjasf/20261128"
)

func TestMergeAliasesSurviveColorChangeAndReimport(t *testing.T) {
	project := marker.NewProject()
	mustNoError(t, project.AddSequence(marker.Sequence{ID: "seq", Name: "Seq", Duration: 100}))
	owner := marker.Owner{Scope: marker.ScopeGlobal, SequenceID: "seq"}
	for _, input := range []marker.Marker{
		{ID: "m-a", Name: "Marker", Owner: owner, Range: marker.FrameRange{Start: 10, End: 20}, Color: "#111111"},
		{ID: "m-b", Name: "Marker", Owner: owner, Range: marker.FrameRange{Start: 15, End: 25}, Color: "#222222"},
	} {
		_, err := project.AddMarker(input)
		mustNoError(t, err)
	}

	_, err := project.Merge([]string{"m-b", "m-a"}, marker.MergeOptions{RequireOverlap: true})
	requireErrorIs(t, err, marker.ErrColorConflict)

	merged, err := project.Merge([]string{"m-b", "m-a"}, marker.MergeOptions{
		Color: "#00FF00", RequireOverlap: true,
	})
	mustNoError(t, err)
	if merged.ID != "m-a" {
		t.Fatalf("canonical id = %q, want m-a", merged.ID)
	}
	if merged.Range != (marker.FrameRange{Start: 10, End: 25}) {
		t.Fatalf("merged range = %#v", merged.Range)
	}

	updated, err := project.SetColor("m-b", "#0000FF")
	mustNoError(t, err)
	if updated.ID != "m-a" {
		t.Fatalf("color update changed identity to %q", updated.ID)
	}
	if updated.Color != "#0000FF" {
		t.Fatalf("color = %q, want #0000FF", updated.Color)
	}
	if len(updated.ColorHistory) < 2 {
		t.Fatalf("color history was not retained: %#v", updated.ColorHistory)
	}

	reloaded, err := marker.Load(project.Export())
	mustNoError(t, err)
	for _, oldID := range []string{"m-a", "m-b"} {
		resolved, err := reloaded.ResolveID(oldID)
		mustNoError(t, err)
		if resolved != "m-a" {
			t.Fatalf("ResolveID(%q) = %q, want m-a", oldID, resolved)
		}
	}
	got, err := reloaded.Get("m-b")
	mustNoError(t, err)
	if got.ID != "m-a" || got.Color != "#0000FF" {
		t.Fatalf("reimported marker = %#v", got)
	}
	if got.Range != (marker.FrameRange{Start: 10, End: 25}) {
		t.Fatalf("reimported range = %#v", got.Range)
	}
	if len(reloaded.Export().Aliases) != 1 {
		t.Fatalf("aliases = %#v, want one alias", reloaded.Export().Aliases)
	}
}

func TestMergeRequiresExplicitNameAndConnectedRanges(t *testing.T) {
	project := marker.NewProject()
	mustNoError(t, project.AddSequence(marker.Sequence{ID: "seq", Name: "Seq", Duration: 100}))
	owner := marker.Owner{Scope: marker.ScopeGlobal, SequenceID: "seq"}
	for _, input := range []marker.Marker{
		{ID: "m-a", Name: "First", Owner: owner, Range: marker.FrameRange{Start: 1, End: 5}, Color: "#111111"},
		{ID: "m-b", Name: "Second", Owner: owner, Range: marker.FrameRange{Start: 20, End: 25}, Color: "#111111"},
	} {
		_, err := project.AddMarker(input)
		mustNoError(t, err)
	}

	_, err := project.Merge([]string{"m-a", "m-b"}, marker.MergeOptions{RequireOverlap: true})
	requireErrorIs(t, err, marker.ErrMergeConflict)
	_, err = project.Merge([]string{"m-a", "m-b"}, marker.MergeOptions{Name: "Merged"})
	mustNoError(t, err)
}

func TestAliasCycleIsRejectedOnImport(t *testing.T) {
	doc := marker.Document{
		SchemaVersion: marker.SchemaVersion,
		Media:         []marker.MediaAsset{{ID: "media", Name: "Media", Duration: 10}},
		Sequences:     []marker.Sequence{{ID: "seq", Name: "Seq", Duration: 10}},
		Markers: []marker.Marker{{
			ID: "m-live", Name: "M",
			Owner: marker.Owner{Scope: marker.ScopeGlobal, SequenceID: "seq"},
			Range: marker.FrameRange{Start: 1, End: 2}, Color: "#111111", Revision: 1,
		}},
		Aliases: []marker.Alias{{From: "old-a", To: "old-b"}, {From: "old-b", To: "old-a"}},
	}
	_, err := marker.Load(doc)
	var validation *marker.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("Load() error = %v, want ValidationError", err)
	}
	if !hasDiagnostic(validation, "alias_cycle") {
		t.Fatalf("diagnostics = %#v, want alias_cycle", validation.Diagnostics)
	}
}

func hasDiagnostic(validation *marker.ValidationError, code string) bool {
	for _, diagnostic := range validation.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

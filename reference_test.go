package marker_test

import (
	"errors"
	"testing"

	marker "github.com/lsafjasf/20261128"
)

func TestImportReportsClipMarkerInWrongSequence(t *testing.T) {
	doc := marker.Document{
		SchemaVersion: marker.SchemaVersion,
		Media:         []marker.MediaAsset{{ID: "media", Name: "Media", Duration: 100}},
		Sequences: []marker.Sequence{
			{ID: "seq-a", Name: "A", Duration: 100},
			{ID: "seq-b", Name: "B", Duration: 100},
		},
		Clips: []marker.Clip{{
			ID: "clip-b", SequenceID: "seq-b", MediaID: "media",
			PlacementStart: 0, SourceIn: 0, SourceOut: 20,
		}},
		Markers: []marker.Marker{{
			ID: "m-clip", Name: "Audio",
			Owner: marker.Owner{Scope: marker.ScopeClip, SequenceID: "seq-a", ClipID: "clip-b"},
			Range: marker.FrameRange{Start: 1, End: 2}, Color: "#112233", Revision: 1,
		}},
	}

	_, err := marker.Load(doc)
	var validation *marker.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("Load() error = %v, want ValidationError", err)
	}
	if !hasDiagnostic(validation, "cross_sequence_reference") {
		t.Fatalf("diagnostics = %#v, want cross_sequence_reference", validation.Diagnostics)
	}
	found := false
	for _, diagnostic := range validation.Diagnostics {
		if diagnostic.Code == "cross_sequence_reference" &&
			diagnostic.MarkerID == "m-clip" &&
			diagnostic.SequenceID == "seq-a" &&
			diagnostic.ClipID == "clip-b" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cross-sequence diagnostic lost its reference fields: %#v", validation.Diagnostics)
	}
}

func TestResolveRejectsUnsolvableCrossSequenceReferences(t *testing.T) {
	project := marker.NewProject()
	mustNoError(t, project.AddMedia(marker.MediaAsset{ID: "media", Name: "Media", Duration: 100}))
	mustNoError(t, project.AddSequence(marker.Sequence{ID: "seq-a", Name: "A", Duration: 100}))
	mustNoError(t, project.AddSequence(marker.Sequence{ID: "seq-b", Name: "B", Duration: 100}))
	mustNoError(t, project.AddClip(marker.Clip{
		ID: "clip-a", SequenceID: "seq-a", MediaID: "media",
		PlacementStart: 0, SourceIn: 0, SourceOut: 20,
	}))
	ownerGlobal := marker.Owner{Scope: marker.ScopeGlobal, SequenceID: "seq-a"}
	ownerClip := marker.Owner{Scope: marker.ScopeClip, SequenceID: "seq-a", ClipID: "clip-a"}
	ownerMedia := marker.Owner{Scope: marker.ScopeMedia, MediaID: "media"}
	for _, input := range []marker.Marker{
		{ID: "m-global", Name: "Global", Owner: ownerGlobal, Range: marker.FrameRange{Start: 1, End: 2}, Color: "#111111"},
		{ID: "m-clip", Name: "Clip", Owner: ownerClip, Range: marker.FrameRange{Start: 1, End: 2}, Color: "#222222"},
		{ID: "m-media", Name: "Media", Owner: ownerMedia, Range: marker.FrameRange{Start: 1, End: 2}, Color: "#333333"},
	} {
		_, err := project.AddMarker(input)
		mustNoError(t, err)
	}

	for _, markerID := range []string{"m-global", "m-clip"} {
		_, err := project.Resolve(marker.Reference{MarkerID: markerID, SequenceID: "seq-b"})
		requireErrorIs(t, err, marker.ErrCrossSequence)
	}
	_, err := project.Resolve(marker.Reference{MarkerID: "m-media", SequenceID: "seq-b"})
	requireErrorIs(t, err, marker.ErrCrossSequence)

	instances, err := project.Resolve(marker.Reference{MarkerID: "m-media", SequenceID: "seq-a"})
	mustNoError(t, err)
	if len(instances) != 1 || instances[0].ClipID != "clip-a" {
		t.Fatalf("media marker instances = %#v", instances)
	}
}

func TestImportCollectsIndependentReferenceProblems(t *testing.T) {
	doc := marker.Document{
		SchemaVersion: marker.SchemaVersion,
		Media:         []marker.MediaAsset{{ID: "media", Name: "Media", Duration: 100}},
		Sequences:     []marker.Sequence{{ID: "seq", Name: "Seq", Duration: 100}},
		Markers: []marker.Marker{
			{
				ID: "m-missing-media", Name: "Media",
				Owner: marker.Owner{Scope: marker.ScopeMedia, MediaID: "missing"},
				Range: marker.FrameRange{Start: 0, End: 1}, Color: "#111111", Revision: 1,
			},
			{
				ID: "m-missing-clip", Name: "Clip",
				Owner: marker.Owner{Scope: marker.ScopeClip, SequenceID: "seq", ClipID: "missing"},
				Range: marker.FrameRange{Start: 0, End: 1}, Color: "#222222", Revision: 1,
			},
			{
				ID: "m-missing-sequence", Name: "Global",
				Owner: marker.Owner{Scope: marker.ScopeGlobal, SequenceID: "missing"},
				Range: marker.FrameRange{Start: 0, End: 1}, Color: "#333333", Revision: 1,
			},
		},
	}
	_, err := marker.Load(doc)
	var validation *marker.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("Load() error = %v, want ValidationError", err)
	}
	for _, code := range []string{"missing_media", "missing_clip", "missing_sequence"} {
		if !hasDiagnostic(validation, code) {
			t.Errorf("missing diagnostic %q in %#v", code, validation.Diagnostics)
		}
	}
}

func TestFailedImportDoesNotReplaceExistingProject(t *testing.T) {
	project := demoProject(t)
	bad := project.Export()
	bad.Aliases = []marker.Alias{{From: "retired", To: "also-missing"}}
	if err := project.Import(bad); err == nil {
		t.Fatal("Import() succeeded, want validation error")
	}
	got, err := project.Get("m-global-color-a")
	mustNoError(t, err)
	if got.Name != "Color" {
		t.Fatalf("existing project was partially replaced: %#v", got)
	}
	if len(project.Export().Markers) != 5 {
		t.Fatalf("existing markers were lost after failed import")
	}
}

package marker_test

import (
	"encoding/json"
	"errors"
	"testing"

	marker "github.com/lsafjasf/20261128"
)

func demoProject(t *testing.T) *marker.Project {
	t.Helper()
	project := marker.NewProject()
	mustNoError(t, project.AddMedia(marker.MediaAsset{ID: "media-main", Name: "Main", Duration: 120}))
	mustNoError(t, project.AddSequence(marker.Sequence{ID: "seq-main", Name: "Main", Duration: 240}))
	mustNoError(t, project.AddClip(marker.Clip{
		ID: "clip-audio", SequenceID: "seq-main", MediaID: "media-main",
		PlacementStart: 40, SourceIn: 10, SourceOut: 80,
	}))

	global := marker.Owner{Scope: marker.ScopeGlobal, SequenceID: "seq-main"}
	clip := marker.Owner{Scope: marker.ScopeClip, SequenceID: "seq-main", ClipID: "clip-audio"}
	media := marker.Owner{Scope: marker.ScopeMedia, MediaID: "media-main"}
	inputs := []marker.Marker{
		{ID: "m-global-color-a", Name: "Color", Owner: global, Range: marker.FrameRange{Start: 42, End: 43}, Color: "#E53935"},
		{ID: "m-global-color-b", Name: "Color", Owner: global, Range: marker.FrameRange{Start: 42, End: 43}, Color: "#1E88E5"},
		{ID: "m-clip-audio-a", Name: "Audio", Owner: clip, Range: marker.FrameRange{Start: 2, End: 3}, Color: "#43A047"},
		{ID: "m-media-audio-a", Name: "Audio", Owner: media, Range: marker.FrameRange{Start: 12, End: 13}, Color: "#FB8C00"},
		{ID: "m-media-audio-b", Name: "Audio", Owner: media, Range: marker.FrameRange{Start: 12, End: 13}, Color: "#8E24AA"},
	}
	for _, input := range inputs {
		_, err := project.AddMarker(input)
		mustNoError(t, err)
	}
	return project
}

func TestSameFrameAndNameKeepsAllMarkerIdentities(t *testing.T) {
	project := demoProject(t)
	doc := project.Export()
	if got, want := len(doc.Markers), 5; got != want {
		t.Fatalf("exported markers = %d, want %d", got, want)
	}

	encoded, err := json.Marshal(doc)
	mustNoError(t, err)
	var decoded marker.Document
	mustNoError(t, json.Unmarshal(encoded, &decoded))
	reloaded, err := marker.Load(decoded)
	mustNoError(t, err)

	instances, err := reloaded.At("seq-main", 42)
	mustNoError(t, err)
	if got, want := len(instances), 5; got != want {
		t.Fatalf("markers at frame 42 = %d, want %d", got, want)
	}

	ids := make(map[string]bool)
	names := make(map[string]int)
	for _, instance := range instances {
		ids[instance.Marker.ID] = true
		names[instance.Marker.Name]++
	}
	if len(ids) != 5 {
		t.Fatalf("unique marker ids = %d, want 5", len(ids))
	}
	if names["Color"] != 2 || names["Audio"] != 3 {
		t.Fatalf("same-name counts = %#v, want Color:2 Audio:3", names)
	}

	for _, id := range []string{
		"m-global-color-a", "m-global-color-b", "m-clip-audio-a",
		"m-media-audio-a", "m-media-audio-b",
	} {
		resolved, err := reloaded.ResolveID(id)
		mustNoError(t, err)
		if resolved != id {
			t.Fatalf("ResolveID(%q) = %q, want itself", id, resolved)
		}
	}
}

func TestOverlapDoesNotImplicitlyMerge(t *testing.T) {
	project := marker.NewProject()
	mustNoError(t, project.AddSequence(marker.Sequence{ID: "seq", Name: "Seq", Duration: 100}))
	owner := marker.Owner{Scope: marker.ScopeGlobal, SequenceID: "seq"}
	for _, input := range []marker.Marker{
		{ID: "m-a", Name: "Color", Owner: owner, Range: marker.FrameRange{Start: 10, End: 20}, Color: "#111111"},
		{ID: "m-b", Name: "Color", Owner: owner, Range: marker.FrameRange{Start: 11, End: 25}, Color: "#222222"},
		{ID: "m-c", Name: "Audio", Owner: owner, Range: marker.FrameRange{Start: 5, End: 12}, Color: "#333333"},
	} {
		_, err := project.AddMarker(input)
		mustNoError(t, err)
	}

	overlapping, err := project.Overlapping(owner, marker.FrameRange{Start: 10, End: 12})
	mustNoError(t, err)
	if got, want := len(overlapping), 3; got != want {
		t.Fatalf("overlapping markers = %d, want %d", got, want)
	}
	if got, want := len(project.Export().Markers), 3; got != want {
		t.Fatalf("exported markers = %d, want %d", got, want)
	}
}

func TestIdenticalVisibleAttributesStillHaveDistinctIdentity(t *testing.T) {
	project := marker.NewProject()
	mustNoError(t, project.AddSequence(marker.Sequence{ID: "seq", Name: "Seq", Duration: 10}))
	owner := marker.Owner{Scope: marker.ScopeGlobal, SequenceID: "seq"}
	for _, id := range []string{"m-first", "m-second"} {
		_, err := project.AddMarker(marker.Marker{
			ID: id, Name: "Same", Owner: owner,
			Range: marker.FrameRange{Start: 4, End: 5}, Color: "#ABCDEF",
		})
		mustNoError(t, err)
	}
	reloaded, err := marker.Load(project.Export())
	mustNoError(t, err)
	instances, err := reloaded.At("seq", 4)
	mustNoError(t, err)
	if len(instances) != 2 {
		t.Fatalf("instances = %d, want 2 markers with identical visible attributes", len(instances))
	}
}

func TestMediaMarkerProjectsThroughEveryMatchingClip(t *testing.T) {
	project := marker.NewProject()
	mustNoError(t, project.AddMedia(marker.MediaAsset{ID: "media", Name: "Media", Duration: 100}))
	mustNoError(t, project.AddSequence(marker.Sequence{ID: "seq", Name: "Seq", Duration: 100}))
	for _, clip := range []marker.Clip{
		{ID: "c1", SequenceID: "seq", MediaID: "media", PlacementStart: 0, SourceIn: 10, SourceOut: 20},
		{ID: "c2", SequenceID: "seq", MediaID: "media", PlacementStart: 50, SourceIn: 10, SourceOut: 20},
	} {
		mustNoError(t, project.AddClip(clip))
	}
	_, err := project.AddMarker(marker.Marker{
		ID: "m-media", Name: "Audio",
		Owner: marker.Owner{Scope: marker.ScopeMedia, MediaID: "media"},
		Range: marker.FrameRange{Start: 12, End: 14}, Color: "#ABCDEF",
	})
	mustNoError(t, err)

	instances, err := project.Resolve(marker.Reference{MarkerID: "m-media", SequenceID: "seq"})
	mustNoError(t, err)
	if got, want := len(instances), 2; got != want {
		t.Fatalf("media instances = %d, want %d", got, want)
	}
	if instances[0].Sequence != (marker.FrameRange{Start: 2, End: 4}) {
		t.Fatalf("first instance = %#v", instances[0].Sequence)
	}
	if instances[1].Sequence != (marker.FrameRange{Start: 52, End: 54}) {
		t.Fatalf("second instance = %#v", instances[1].Sequence)
	}
}

func mustNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func requireErrorIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want errors.Is(%v)", err, target)
	}
}

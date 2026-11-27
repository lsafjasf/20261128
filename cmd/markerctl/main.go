package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	marker "github.com/lsafjasf/20261128"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		var validation *marker.ValidationError
		if errors.As(err, &validation) {
			_ = writeJSON(os.Stdout, validation.Diagnostics)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "markerctl:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: markerctl <demo|validate|resolve> ...")
	}
	switch args[0] {
	case "demo":
		if len(args) != 1 {
			return errors.New("usage: markerctl demo")
		}
		project, err := buildDemo()
		if err != nil {
			return err
		}
		return writeJSON(stdout, project.Export())
	case "validate":
		if len(args) != 2 {
			return errors.New("usage: markerctl validate <export.json>")
		}
		project, err := readProject(args[1])
		if err != nil {
			return err
		}
		doc := project.Export()
		fmt.Fprintf(stderr, "valid: %d markers, %d aliases, schema %d\n",
			len(doc.Markers), len(doc.Aliases), doc.SchemaVersion)
		return nil
	case "resolve":
		if len(args) < 3 || len(args) > 4 {
			return errors.New("usage: markerctl resolve <export.json> <marker-id> [sequence-id]")
		}
		project, err := readProject(args[1])
		if err != nil {
			return err
		}
		ref := marker.Reference{MarkerID: args[2]}
		if len(args) == 4 {
			ref.SequenceID = args[3]
		}
		instances, err := project.Resolve(ref)
		if err != nil {
			return err
		}
		return writeJSON(stdout, instances)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func readProject(path string) (*marker.Project, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var doc marker.Document
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return marker.Load(doc)
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func buildDemo() (*marker.Project, error) {
	project := marker.NewProject()
	if err := project.AddMedia(marker.MediaAsset{ID: "media-main", Name: "Main media", Duration: 120}); err != nil {
		return nil, err
	}
	if err := project.AddSequence(marker.Sequence{ID: "seq-main", Name: "Main sequence", Duration: 240}); err != nil {
		return nil, err
	}
	if err := project.AddClip(marker.Clip{
		ID: "clip-audio", SequenceID: "seq-main", MediaID: "media-main",
		PlacementStart: 40, SourceIn: 10, SourceOut: 80,
	}); err != nil {
		return nil, err
	}

	globalOwner := marker.Owner{Scope: marker.ScopeGlobal, SequenceID: "seq-main"}
	clipOwner := marker.Owner{Scope: marker.ScopeClip, SequenceID: "seq-main", ClipID: "clip-audio"}
	mediaOwner := marker.Owner{Scope: marker.ScopeMedia, MediaID: "media-main"}
	inputs := []marker.Marker{
		{ID: "m-global-color-a", Name: "Color", Owner: globalOwner, Range: marker.FrameRange{Start: 42, End: 43}, Color: "#E53935"},
		{ID: "m-global-color-b", Name: "Color", Owner: globalOwner, Range: marker.FrameRange{Start: 42, End: 43}, Color: "#1E88E5"},
		{ID: "m-clip-audio-a", Name: "Audio", Owner: clipOwner, Range: marker.FrameRange{Start: 2, End: 3}, Color: "#43A047"},
		{ID: "m-media-audio-a", Name: "Audio", Owner: mediaOwner, Range: marker.FrameRange{Start: 12, End: 13}, Color: "#FB8C00"},
		{ID: "m-media-audio-b", Name: "Audio", Owner: mediaOwner, Range: marker.FrameRange{Start: 12, End: 13}, Color: "#8E24AA"},
	}
	for _, input := range inputs {
		if _, err := project.AddMarker(input); err != nil {
			return nil, err
		}
	}

	instances, err := project.At("seq-main", 42)
	if err != nil {
		return nil, err
	}
	if len(instances) != len(inputs) {
		return nil, fmt.Errorf("demo invariant failed: expected %d markers at frame 42, got %d", len(inputs), len(instances))
	}
	reloaded, err := marker.Load(project.Export())
	if err != nil {
		return nil, err
	}
	return reloaded, nil
}

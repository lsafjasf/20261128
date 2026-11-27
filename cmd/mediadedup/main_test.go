package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mediadedup/dedup"
)

func TestRunFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "pool.json")
	outPath := filepath.Join(dir, "result.json")

	input := map[string]any{
		"assets": []map[string]any{
			{
				"id": "a1", "path": "a1.mov", "duration": 10.0,
				"width": 1920, "height": 1080, "frameRate": 30.0, "rotation": 0,
				"codec": "prores", "bitrateK": 40000,
				"video": map[string]any{"rotation": 0, "frames": []uint64{1, 2, 3, 4}},
				"audio": map[string]any{"points": []uint64{9, 8, 7, 6}},
			},
			{
				"id": "a2", "path": "a2.mp4", "duration": 10.0,
				"width": 1080, "height": 1920, "frameRate": 24.0, "rotation": 90,
				"codec": "h264", "bitrateK": 8000,
				"video": map[string]any{"rotation": 90, "frames": []uint64{1, 2, 3, 4}},
				"audio": map[string]any{"points": []uint64{9, 8, 7, 6}},
			},
			{
				"id": "a3", "path": "a3.mov", "duration": 10.0,
				"width": 1920, "height": 1080, "frameRate": 30.0, "rotation": 0,
				"codec": "h264", "bitrateK": 6000,
				"video": map[string]any{"rotation": 0,
					"frames": []uint64{^uint64(0), ^uint64(1), ^uint64(2), ^uint64(3)}},
				"audio": map[string]any{"points": []uint64{^uint64(0), ^uint64(1), ^uint64(2), ^uint64(3)}},
			},
		},
		"clips": []map[string]any{
			{"id": "c1", "trackId": "V1", "assetId": "a2", "start": 0.0, "sourceIn": 0.0, "sourceOut": 4.0},
			{"id": "c2", "trackId": "V1", "assetId": "a3", "start": 4.0, "sourceIn": 0.0, "sourceOut": 4.0},
		},
		// Keyed by member asset ID to exercise the flexible lookup.
		"masters": map[string]string{"a2": "a1"},
	}

	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runFile(inPath, outPath); err != nil {
		t.Fatalf("runFile: %v", err)
	}

	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var result outputFile
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("groups=%d want 1", len(result.Groups))
	}
	g := result.Groups[0]
	if g.MasterID != "a1" {
		t.Fatalf("master=%s want user-selected a1", g.MasterID)
	}
	if len(g.Members) != 2 {
		t.Fatalf("members=%v want a1,a2 only (a3 is a different shot)", g.Members)
	}
	if len(result.Singletons) != 1 || result.Singletons[0] != "a3" {
		t.Fatalf("singletons=%v want [a3]", result.Singletons)
	}

	// c1 belongs to the group and is swapped; c2 is a different shot, untouched.
	clips := map[string]string{}
	for _, c := range result.Clips {
		clips[c.ID] = c.AssetID
	}
	if clips["c1"] != "a1" {
		t.Fatalf("c1=%s want master a1", clips["c1"])
	}
	if clips["c2"] != "a3" {
		t.Fatalf("c2=%s must stay a3 (different group)", clips["c2"])
	}
	if len(result.Reports[0].Replacements) != 1 {
		t.Fatalf("replacements=%d want 1", len(result.Reports[0].Replacements))
	}
}

func TestRelationJSON(t *testing.T) {
	for want, s := range map[dedup.Relation]string{
		dedup.RelationIdentical: "identical",
		dedup.RelationTranscode: "transcode",
		dedup.RelationDifferent: "different",
	} {
		raw, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != `"`+s+`"` {
			t.Fatalf("json=%s want %q", raw, s)
		}
		var got dedup.Relation
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("roundtrip=%v want %v", got, want)
		}
	}
}

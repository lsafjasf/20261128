package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"renderpipe"
)

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func TestRenderAndVerifyCommand(t *testing.T) {
	project := t.TempDir()
	frameDir := filepath.Join(project, "frames")
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pngBytes, err := base64.StdEncoding.DecodeString(onePixelPNG)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameDir, "one.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()

	first := renderCommand(t, project, output)
	if first.Reused || first.TaskID == "" {
		t.Fatalf("unexpected first result: %+v", first)
	}
	second := renderCommand(t, project, output)
	if !second.Reused || second.Directory != first.Directory || second.MediaHash != first.MediaHash {
		t.Fatalf("unexpected reused result: %+v", second)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"verify", second.Directory, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("verify exit=%d stderr=%s", code, stderr.String())
	}
	var manifest renderpipe.ArtifactManifest
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.TaskID != first.TaskID || manifest.Media.SHA256 != first.MediaHash {
		t.Fatal("verify output does not describe the rendered artifact")
	}
}

func renderCommand(t *testing.T, project, output string) renderpipe.RenderResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run([]string{"render", "--project", project, "--output", output, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("render exit=%d stderr=%s", code, stderr.String())
	}
	var result renderpipe.RenderResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

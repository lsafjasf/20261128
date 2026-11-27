package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExample(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"../../examples/release-package/release.json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"发行矩阵", "theatrical", "streaming", "mobile", "0 个错误"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

func TestRunBlocksOnMissingVersion(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "release.json")
	data := `{
	  "schemaVersion": 1,
	  "versions": [{
	    "id": "theatrical",
	    "window": {"channel": "theatrical", "start": "2026-12-01", "end": "2027-01-01"},
	    "video": {"path": "a.mxf", "aspectRatio": "2.39:1", "width": 4096, "height": 1716},
	    "audio": {"path": "a.wav", "layout": "5.1", "channels": 6},
	    "subtitles": [{"path": "a.srt", "language": "en", "kind": "full"}]
	  }]
	}`
	if err := os.WriteFile(manifest, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"-skip-files", manifest}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "missing_version") {
		t.Fatalf("missing_version not in output:\n%s", stdout.String())
	}
}

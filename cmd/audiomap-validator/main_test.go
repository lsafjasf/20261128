package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lsafjasf/20261128/channelmap"
)

func writeTestInputs(t *testing.T, deliveryJSON string) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	containerPath := filepath.Join(dir, "container.json")
	deliveryPath := filepath.Join(dir, "delivery.json")
	containerJSON := `{
		"format": "MOV",
		"codec": "pcm_s24le",
		"channel_count": 6,
		"channel_order": ["FL", "FR", "FC", "LFE", "BL", "BR"]
	}`
	if err := os.WriteFile(containerPath, []byte(containerJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deliveryPath, []byte(deliveryJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, containerPath, deliveryPath
}

func TestRunWritesEvidence(t *testing.T) {
	dir, containerPath, deliveryPath := writeTestInputs(t, `{
		"name": "broadcast-5.1",
		"channels": ["L", "R", "Ls", "Rs", "C", "LFE"]
	}`)
	outDir := filepath.Join(dir, "evidence")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"validate",
		"--container", containerPath,
		"--delivery", deliveryPath,
		"--out", outDir,
		"--json",
	}, &stdout, &stderr)
	if code != exitValid {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}

	var result successResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON result: %v\n%s", err, stdout.String())
	}
	if !result.Valid || result.Identity {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, name := range []string{
		channelmap.MappingFileName,
		channelmap.MappingCSVName,
		channelmap.ReadbackFileName,
	} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("missing evidence file %s: %v", name, err)
		}
	}

	readbackData, err := os.ReadFile(filepath.Join(outDir, channelmap.ReadbackFileName))
	if err != nil {
		t.Fatal(err)
	}
	var readback channelmap.ReadbackEvidence
	if err := json.Unmarshal(readbackData, &readback); err != nil {
		t.Fatal(err)
	}
	if !readback.Verified {
		t.Fatal("CLI readback evidence is not verified")
	}
}

func TestRunRejectsOverlap(t *testing.T) {
	dir, containerPath, deliveryPath := writeTestInputs(t, `{
		"channels": ["L", "R", "Ls", "Ls", "C", "LFE"]
	}`)
	outDir := filepath.Join(dir, "evidence")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--container", containerPath,
		"--delivery", deliveryPath,
		"--out", outDir,
		"--json",
	}, &stdout, &stderr)
	if code != exitRejected {
		t.Fatalf("run() code = %d, want %d", code, exitRejected)
	}
	if !strings.Contains(stdout.String(), `"code": "overlap"`) {
		t.Fatalf("stdout does not contain overlap issue: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "rejection.json")); err != nil {
		t.Fatalf("rejection evidence missing: %v", err)
	}
}

func TestRunRejectsIncompleteDeliveryDuringParse(t *testing.T) {
	dir, containerPath, deliveryPath := writeTestInputs(t, `{
		"channels": [
			{"slot": 1, "label": "L", "source": "FL"},
			{"slot": 2, "label": "R"},
			{"slot": 3, "label": "Ls"},
			{"slot": 4, "label": "Rs"},
			{"slot": 5, "label": "C"},
			{"slot": 6, "label": "LFE"}
		]
	}`)
	outDir := filepath.Join(dir, "evidence")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--container", containerPath,
		"--delivery", deliveryPath,
		"--out", outDir,
		"--json",
	}, &stdout, &stderr)
	if code != exitRejected {
		t.Fatalf("run() code = %d, want %d; stderr=%s", code, exitRejected, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"code": "incomplete"`) {
		t.Fatalf("stdout does not contain incomplete issue: %s", stdout.String())
	}
}

func TestRunRequiresInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("run() code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "--container") {
		t.Fatalf("usage text missing: %s", stderr.String())
	}
}

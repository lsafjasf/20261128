package batchout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func testInput(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func testSpec(input string, params map[string]string) TaskSpec {
	return TaskSpec{
		Inputs: []Input{{Name: "source", Path: input}},
		Params: params,
		Recipe: "test-pipeline-v1",
	}
}

func TestRunCommitsAndRecordsProvenance(t *testing.T) {
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source bytes")
	output := filepath.Join(root, "out")
	mgr, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	produce := func(_ context.Context, stage string) error {
		calls.Add(1)
		if err := os.MkdirAll(filepath.Join(stage, "clips"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(stage, "clips", "final.mp4"), []byte("video"), 0o644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage, "manifest.txt"), []byte("meta"), 0o644)
	}

	result, err := mgr.Run(context.Background(), testSpec(input, map[string]string{
		"codec": "h264",
		"crf":   "18",
	}), produce)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("producer calls = %d, want 1", calls.Load())
	}
	if readTestFile(t, filepath.Join(output, "clips", "final.mp4")) != "video" {
		t.Fatal("committed output has wrong content")
	}

	found, err := mgr.Lookup(filepath.Join(output, "clips", "final.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if found.TaskID != result.TaskID || found.Recipe != "test-pipeline-v1" {
		t.Fatalf("unexpected provenance identity: %+v", found)
	}
	if found.Params["codec"] != "h264" || found.Params["crf"] != "18" {
		t.Fatalf("unexpected parameters: %#v", found.Params)
	}
	if len(found.Inputs) != 1 || found.Inputs[0].SHA256 == "" {
		t.Fatalf("unexpected input provenance: %+v", found.Inputs)
	}

	again, err := mgr.Run(context.Background(), testSpec(input, map[string]string{
		"crf":   "18",
		"codec": "h264",
	}), produce)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("committed task ran again: producer calls = %d", calls.Load())
	}
	if again.TaskID != result.TaskID || again.AttemptID != result.AttemptID {
		t.Fatalf("idempotent result changed: first=%+v second=%+v", result, again)
	}
}

func TestExistingManualFilesAreNeverOverwritten(t *testing.T) {
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source")
	output := filepath.Join(root, "out")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	manual := filepath.Join(output, "final.mp4")
	if err := os.WriteFile(manual, []byte("manual final"), 0o644); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(output, "final.mp4.part")
	if err := os.WriteFile(partial, []byte("old partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	produce := func(content string) Producer {
		return func(_ context.Context, stage string) error {
			return os.WriteFile(filepath.Join(stage, "final.mp4"), []byte(content), 0o644)
		}
	}

	first, err := mgr.Run(context.Background(), testSpec(input, map[string]string{"version": "1"}), produce("new one"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := mgr.Run(context.Background(), testSpec(input, map[string]string{"version": "2"}), produce("new two"))
	if err != nil {
		t.Fatal(err)
	}

	if readTestFile(t, manual) != "manual final" {
		t.Fatal("manual final was overwritten")
	}
	if readTestFile(t, partial) != "old partial" {
		t.Fatal("old partial was overwritten")
	}
	if first.Outputs[0].Path == "final.mp4" || second.Outputs[0].Path == "final.mp4" {
		t.Fatalf("collision was not renamed: %q, %q", first.Outputs[0].Path, second.Outputs[0].Path)
	}
	if first.Outputs[0].Path == second.Outputs[0].Path {
		t.Fatalf("different tasks received the same conflict name %q", first.Outputs[0].Path)
	}
	if !strings.HasPrefix(first.Outputs[0].Path, "final.") || !strings.HasSuffix(first.Outputs[0].Path, ".mp4") {
		t.Fatalf("unexpected conflict name %q", first.Outputs[0].Path)
	}
	if readTestFile(t, filepath.Join(output, first.Outputs[0].Path)) != "new one" {
		t.Fatal("first conflict output has wrong content")
	}
	if readTestFile(t, filepath.Join(output, second.Outputs[0].Path)) != "new two" {
		t.Fatal("second conflict output has wrong content")
	}

	for _, result := range []*Result{first, second} {
		if _, err := mgr.Lookup(result.Outputs[0].Path); err != nil {
			t.Fatalf("lookup %s: %v", result.Outputs[0].Path, err)
		}
	}
}

func TestIdenticalManualContentStillGetsConflictName(t *testing.T) {
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source")
	output := filepath.Join(root, "out")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	manual := filepath.Join(output, "final.mp4")
	if err := os.WriteFile(manual, []byte("identical"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	result, err := mgr.Run(context.Background(), testSpec(input, nil), func(_ context.Context, stage string) error {
		return os.WriteFile(filepath.Join(stage, "final.mp4"), []byte("identical"), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs[0].Path == "final.mp4" {
		t.Fatal("pre-existing identical file was misattributed to this task")
	}
	if _, err := mgr.Lookup("final.mp4"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("manual file unexpectedly has provenance: %v", err)
	}
	if _, err := mgr.Lookup(result.Outputs[0].Path); err != nil {
		t.Fatalf("conflict output has no provenance: %v", err)
	}
}

func TestTaskLockRejectsConcurrentRun(t *testing.T) {
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source")
	mgr, err := Open(filepath.Join(root, "out"))
	if err != nil {
		t.Fatal(err)
	}
	spec := testSpec(input, nil)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := mgr.Run(context.Background(), spec, func(_ context.Context, stage string) error {
			close(started)
			<-release
			return os.WriteFile(filepath.Join(stage, "result.txt"), []byte("ok"), 0o644)
		})
		done <- err
	}()
	<-started

	_, err = mgr.Run(context.Background(), spec, func(_ context.Context, _ string) error {
		t.Error("second producer must not run")
		return nil
	})
	if !errors.Is(err, ErrTaskLocked) {
		t.Fatalf("second Run error = %v, want ErrTaskLocked", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first Run: %v", err)
	}
}

package batchout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestManualSymlinkIsNotFollowedOrReplaced(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows hosts")
	}
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source")
	outside := testInput(t, root, "outside.txt", "outside manual data")
	output := filepath.Join(root, "out")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(output, "result.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	mgr, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	result, err := mgr.Run(context.Background(), testSpec(input, nil), func(_ context.Context, stage string) error {
		return os.WriteFile(filepath.Join(stage, "result.txt"), []byte("generated"), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if readTestFile(t, outside) != "outside manual data" {
		t.Fatal("manual symlink target was modified")
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("manual symlink was replaced: mode=%v err=%v", info, err)
	}
	if result.Outputs[0].Path == "result.txt" {
		t.Fatal("symlink collision did not get a conflict name")
	}
	if readTestFile(t, filepath.Join(output, result.Outputs[0].Path)) != "generated" {
		t.Fatal("generated output has wrong content")
	}
}

func TestStagedSymlinkIsRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows hosts")
	}
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source")
	outside := testInput(t, root, "outside.txt", "outside")
	output := filepath.Join(root, "out")
	mgr, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}

	_, err = mgr.Run(context.Background(), testSpec(input, nil), func(_ context.Context, stage string) error {
		return os.Symlink(outside, filepath.Join(stage, "result.txt"))
	})
	if err == nil {
		t.Fatal("staged symlink was accepted")
	}
	if _, statErr := os.Stat(filepath.Join(output, "result.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("staged symlink became visible: %v", statErr)
	}
}

func TestLockSymlinkIsNotFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows hosts")
	}
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source")
	outside := testInput(t, root, "outside.txt", "must not be truncated")
	output := filepath.Join(root, "out")
	mgr, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	taskID, _, _, _, err := fingerprint(testSpec(input, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, mgr.lockPath(taskID)); err != nil {
		t.Fatal(err)
	}

	_, err = mgr.Run(context.Background(), testSpec(input, nil), func(_ context.Context, stage string) error {
		return os.WriteFile(filepath.Join(stage, "result.txt"), []byte("result"), 0o644)
	})
	if err == nil {
		t.Fatal("Run followed a symlinked lock file")
	}
	if readTestFile(t, outside) != "must not be truncated" {
		t.Fatal("lock symlink target was modified")
	}
}

package batchout

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTaskIdentityDoesNotDependOnInputLocation(t *testing.T) {
	root := t.TempDir()
	first := testInput(t, root, "first.bin", "same content")
	secondDir := filepath.Join(root, "moved")
	if err := os.MkdirAll(secondDir, 0o755); err != nil {
		t.Fatal(err)
	}
	second := testInput(t, secondDir, "renamed.bin", "same content")

	id1, _, _, _, err := fingerprint(testSpec(first, map[string]string{"v": "1"}))
	if err != nil {
		t.Fatal(err)
	}
	id2, _, _, _, err := fingerprint(testSpec(second, map[string]string{"v": "1"}))
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("task id changed with input location: %s != %s", id1, id2)
	}

	id3, _, _, _, err := fingerprint(testSpec(second, map[string]string{"v": "2"}))
	if err != nil {
		t.Fatal(err)
	}
	if id3 == id1 {
		t.Fatal("parameter change did not change task id")
	}
}

func TestOpenRecoverySkipsActiveProducer(t *testing.T) {
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source")
	output := filepath.Join(root, "out")
	mgr, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := mgr.Run(context.Background(), testSpec(input, nil), func(_ context.Context, stage string) error {
			close(started)
			<-release
			return os.WriteFile(filepath.Join(stage, "result.txt"), []byte("result"), 0o644)
		})
		done <- err
	}()
	<-started

	journals, err := filepath.Glob(mgr.statePath(stateJournals, "*.json"))
	if err != nil || len(journals) != 1 {
		t.Fatalf("journals = %v, err = %v", journals, err)
	}
	j, err := mgr.readJournal(journals[0])
	if err != nil {
		t.Fatal(err)
	}
	stage := mgr.stagePath(j.TaskID, j.AttemptID)

	// Open performs recovery. The active task lock must make it skip, not
	// delete the producer's staging directory.
	if _, err := Open(output); err != nil {
		t.Fatalf("Open while task is active: %v", err)
	}
	if _, err := os.Stat(stage); err != nil {
		t.Fatalf("active staging directory was removed: %v", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("active Run: %v", err)
	}
}

func TestInternalStateSymlinkIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows hosts")
	}
	root := t.TempDir()
	output := filepath.Join(root, "out")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(output, stateRoot)); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(output); err == nil {
		t.Fatal("Open followed a symlinked .batch directory")
	}
}

func TestRecoverRecreatesMissingCommittedRecord(t *testing.T) {
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source")
	output := filepath.Join(root, "out")
	mgr, err := Open(output, WithoutAutoRecover())
	if err != nil {
		t.Fatal(err)
	}
	j := prepareJournal(t, mgr, input, map[string]string{"result.txt": "committed data"}, StateCommitted)
	source := filepath.Join(mgr.stagePath(j.TaskID, j.AttemptID), "result.txt")
	if err := os.Link(source, filepath.Join(output, "result.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(mgr.recordPath(j.TaskID)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	report, err := mgr.Recover(context.Background())
	if err != nil {
		t.Fatalf("recover: %v (report %+v)", err, report)
	}
	if _, err := mgr.Lookup("result.txt"); err != nil {
		t.Fatalf("record was not reconstructed: %v", err)
	}
}

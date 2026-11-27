package batchout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func prepareJournal(t *testing.T, mgr *Manager, input string, files map[string]string, state State) *journal {
	t.Helper()
	taskID, inputs, paramsDigest, params, err := fingerprint(testSpec(input, map[string]string{"quality": "high"}))
	if err != nil {
		t.Fatal(err)
	}
	attemptID, err := newAttemptID()
	if err != nil {
		t.Fatal(err)
	}
	stage := mgr.stagePath(taskID, attemptID)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		full := filepath.Join(stage, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	outputs, err := snapshotOutputs(stage)
	if err != nil {
		t.Fatal(err)
	}
	for i := range outputs {
		outputs[i].Path = outputs[i].Name
	}
	now := time.Now().UTC()
	j := &journal{
		Provenance: Provenance{
			Version:      Version,
			TaskID:       taskID,
			AttemptID:    attemptID,
			Recipe:       "test-pipeline-v1",
			Inputs:       inputs,
			Params:       params,
			ParamsDigest: paramsDigest,
			Outputs:      outputs,
		},
		State:     state,
		CreatedAt: now,
		UpdatedAt: now,
		StageDir:  filepath.ToSlash(filepath.Join(stateRoot, stateTmp, taskID, attemptID)),
	}
	if err := mgr.saveJournal(j); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestRecoverCompletesPartialCommit(t *testing.T) {
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source")
	output := filepath.Join(root, "out")
	mgr, err := Open(output, WithoutAutoRecover())
	if err != nil {
		t.Fatal(err)
	}
	j := prepareJournal(t, mgr, input, map[string]string{
		"a.mp4": "first",
		"b.mp4": "second",
	}, StateCommitting)

	// Simulate a crash after the first output became visible.
	firstSource := filepath.Join(mgr.stagePath(j.TaskID, j.AttemptID), "a.mp4")
	if err := mgr.placeOutput(j, &j.Outputs[0], firstSource); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, "b.mp4")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("b.mp4 should not exist before recovery: %v", err)
	}

	reopened, err := Open(output, WithoutAutoRecover())
	if err != nil {
		t.Fatal(err)
	}
	report, err := reopened.Recover(context.Background())
	if err != nil {
		t.Fatalf("recover: %v (report %+v)", err, report)
	}
	if len(report.Entries) != 1 || report.Entries[0].Action != RecoveredCommitted {
		t.Fatalf("unexpected recovery report: %+v", report.Entries)
	}
	if readTestFile(t, filepath.Join(output, "a.mp4")) != "first" {
		t.Fatal("first output changed during recovery")
	}
	if readTestFile(t, filepath.Join(output, "b.mp4")) != "second" {
		t.Fatal("second output was not committed during recovery")
	}
	rec, err := reopened.loadRecord(j.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.AttemptID != j.AttemptID || len(rec.Outputs) != 2 || rec.Params["quality"] != "high" {
		t.Fatalf("unexpected recovered provenance: %+v", rec)
	}
	if _, err := os.Stat(mgr.stagePath(j.TaskID, j.AttemptID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging directory was not cleaned: %v", err)
	}
	for _, out := range []string{"a.mp4", "b.mp4"} {
		if _, err := reopened.Lookup(out); err != nil {
			t.Fatalf("lookup %s after recovery: %v", out, err)
		}
	}
}

func TestRecoverRollsBackBeforeCommit(t *testing.T) {
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source")
	output := filepath.Join(root, "out")
	mgr, err := Open(output, WithoutAutoRecover())
	if err != nil {
		t.Fatal(err)
	}
	j := prepareJournal(t, mgr, input, map[string]string{"never.mp4": "partial"}, StatePrepared)
	stage := mgr.stagePath(j.TaskID, j.AttemptID)

	report, err := mgr.Recover(context.Background())
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if len(report.Entries) != 1 || report.Entries[0].Action != RecoveredRolledBack {
		t.Fatalf("unexpected recovery report: %+v", report.Entries)
	}
	if _, err := os.Stat(filepath.Join(output, "never.mp4")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partially staged file became visible: %v", err)
	}
	if _, err := os.Stat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stage was not removed: %v", err)
	}
	latest, err := mgr.readJournal(mgr.journalPath(j.TaskID, j.AttemptID))
	if err != nil {
		t.Fatal(err)
	}
	if latest.State != StateAborted {
		t.Fatalf("journal state = %s, want aborted", latest.State)
	}
}

func TestCollisionAppearingAfterPlanningGetsConflictName(t *testing.T) {
	root := t.TempDir()
	input := testInput(t, root, "input.bin", "source")
	output := filepath.Join(root, "out")
	mgr, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	j := prepareJournal(t, mgr, input, map[string]string{"report.txt": "generated"}, StatePrepared)

	manual := filepath.Join(output, "report.txt")
	if err := os.WriteFile(manual, []byte("manual appeared late"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := mgr.commitJournal(j)
	if err != nil {
		t.Fatal(err)
	}
	if readTestFile(t, manual) != "manual appeared late" {
		t.Fatal("late manual file was overwritten")
	}
	if result.Outputs[0].Path == "report.txt" {
		t.Fatal("collision did not receive a conflict name")
	}
	if readTestFile(t, filepath.Join(output, result.Outputs[0].Path)) != "generated" {
		t.Fatal("conflict output has wrong content")
	}
}

package demo

import (
	"os"
	"testing"

	"editorautosave/project"
)

func TestRunEndToEnd(t *testing.T) {
	root := t.TempDir()
	summary, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}

	if summary.DiskSeqBeforeCrash != 2 || summary.HeadSeqBeforeCrash != 3 {
		t.Fatalf("unexpected seqs: %+v", summary)
	}
	if summary.LastSuccessSeq != 3 {
		t.Fatalf("last success seq = %d, want 3", summary.LastSuccessSeq)
	}

	recovered, err := project.Load(summary.RecoveredFirst)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.LastSeq() != 3 {
		t.Fatalf("first recovery seq = %d, want 3", recovered.LastSeq())
	}

	if summary.BreakSnapshotID == "" {
		t.Fatal("expected a recorded chain break")
	}
	if summary.FallbackSeq != 2 {
		t.Fatalf("fallback seq = %d, want 2", summary.FallbackSeq)
	}
	if !summary.OriginalUntouched {
		t.Fatal("original project was modified during recovery")
	}
	if len(summary.MissingMedia) == 0 {
		t.Fatal("expected missing media warning after deleting clip-b.mp4")
	}

	// Recovered files sit beside the untouched original, never replacing it.
	if _, err := os.Stat(summary.DiskProjectPath); err != nil {
		t.Fatalf("original project missing: %v", err)
	}
}

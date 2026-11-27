package autosave

import (
	"os"
	"path/filepath"
	"testing"

	"editorautosave/project"
)

// Reopening after a head corruption must quarantine the bad file and keep
// the chain pointing at the last good snapshot.
func TestReopenAfterHeadCorruption(t *testing.T) {
	dir := t.TempDir()
	gen := seqID()
	store, _ := Open(dir, WithIDGen(gen), WithBaseEvery(100))
	p := newTestProject(t)
	_ = p.Apply(project.Operation{Seq: 1, Kind: project.OpAddClip, ClipID: "c1", MediaID: "m1", Start: 0, End: 5})
	if _, err := store.SaveBase(p); err != nil {
		t.Fatal(err)
	}
	_ = p.Apply(project.Operation{Seq: 2, Kind: project.OpMoveClip, ClipID: "c1", Track: 1, Position: 2})
	head, err := store.AppendIncremental(p)
	if err != nil {
		t.Fatal(err)
	}

	bad := filepath.Join(dir, store.snapRel(head))
	if err := os.WriteFile(bad, []byte("nonsense"), 0o644); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, WithIDGen(gen), WithBaseEvery(100))
	if err != nil {
		t.Fatal(err)
	}
	m, _ := reopened.Manifest()
	if m.Head.Seq != 1 || m.LastSuccessSeq != 1 {
		t.Fatalf("head must roll back to seq 1, got head=%d last=%d", m.Head.Seq, m.LastSuccessSeq)
	}
	if _, err := os.Stat(filepath.Join(dir, archiveDir, filepath.Base(bad))); err != nil {
		t.Fatalf("corrupt head not archived: %v", err)
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("corrupt head file must be moved away from active chain")
	}
}

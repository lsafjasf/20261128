package autosave

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"editorautosave/project"
)

func newTestProject(t *testing.T) *project.Project {
	t.Helper()
	p := project.New("proj-1")
	p.RegisterMedia(project.MediaRef{ID: "m1", Path: "a.mp4", Size: 10, Hash: "abc"})
	p.RegisterMedia(project.MediaRef{ID: "m2", Path: "b.mp4", Size: 20, Hash: "def"})
	return p
}

func TestBaseIncrementalChainAndReplay(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, WithIDGen(seqID()))
	if err != nil {
		t.Fatal(err)
	}
	p := newTestProject(t)

	if err := p.Apply(project.Operation{Seq: 1, Kind: project.OpAddClip, ClipID: "c1", MediaID: "m1", Start: 0, End: 5}); err != nil {
		t.Fatal(err)
	}
	base, err := store.SaveBase(p)
	if err != nil {
		t.Fatal(err)
	}
	if base.Kind != KindBase || base.Seq != 1 {
		t.Fatalf("unexpected base: %+v", base)
	}

	for seq := 2; seq <= 4; seq++ {
		if err := p.Apply(project.Operation{
			Seq: seq, Kind: project.OpAddClip,
			ClipID: "c" + string(rune('0'+seq)), MediaID: "m2",
			Track: seq, Start: 0, End: 2, Position: float64(seq),
		}); err != nil {
			t.Fatal(err)
		}
		snap, err := store.AppendIncremental(p)
		if err != nil {
			t.Fatalf("incremental seq %d: %v", seq, err)
		}
		if snap.Kind != KindIncremental || snap.Seq != seq || len(snap.Operations) != 1 {
			t.Fatalf("bad incremental: %+v", snap)
		}
	}

	manifest, _ := store.Manifest()
	if manifest.LastSuccessSeq != 4 || manifest.Head.Seq != 4 {
		t.Fatalf("manifest head wrong: %+v", manifest)
	}

	replayed, err := store.Replay(manifest.Snapshots)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.LastSeq() != 4 || replayed.StateHash() != p.StateHash() {
		t.Fatal("replayed state diverges from live project")
	}

	// Reopening reconciles and exposes the identical chain.
	reopened, err := Open(dir, WithIDGen(seqID()))
	if err != nil {
		t.Fatal(err)
	}
	m2, _ := reopened.Manifest()
	if len(m2.Snapshots) != 4 {
		t.Fatalf("snapshot count after reopen = %d", len(m2.Snapshots))
	}
}

func TestAppendRequiresContiguousOps(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(dir, WithIDGen(seqID()))
	p := newTestProject(t)
	_ = p.Apply(project.Operation{Seq: 1, Kind: project.OpAddClip, ClipID: "c1", MediaID: "m1", Start: 0, End: 5})
	if _, err := store.SaveBase(p); err != nil {
		t.Fatal(err)
	}

	gap := p.Clone()
	gap.Operations = append(gap.Operations, project.Operation{Seq: 3, Kind: project.OpMoveClip, ClipID: "c1", Track: 1, Position: 2})
	if _, err := store.AppendIncremental(gap); !errors.Is(err, project.ErrSeqGap) {
		t.Fatalf("expected ErrSeqGap, got %v", err)
	}
	manifest, _ := store.Manifest()
	if manifest.Head.Seq != 1 || manifest.LastSuccessSeq != 1 {
		t.Fatalf("failed append must not advance the chain: %+v", manifest)
	}
}

func TestCompactionArchivesOldChain(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(dir, WithIDGen(seqID()), WithBaseEvery(2))
	p := newTestProject(t)
	_ = p.Apply(project.Operation{Seq: 1, Kind: project.OpAddClip, ClipID: "c1", MediaID: "m1", Start: 0, End: 5})
	if _, err := store.SaveBase(p); err != nil {
		t.Fatal(err)
	}
	for seq := 2; seq <= 3; seq++ {
		_ = p.Apply(project.Operation{Seq: seq, Kind: project.OpMoveClip, ClipID: "c1", Track: seq, Position: float64(seq)})
		if _, err := store.AppendIncremental(p); err != nil {
			t.Fatal(err)
		}
	}
	manifest, _ := store.Manifest()
	if len(manifest.Snapshots) != 1 || manifest.Base.Seq != 3 {
		t.Fatalf("expected compacted single base, got %+v", manifest.Snapshots)
	}
	entries, err := os.ReadDir(filepath.Join(dir, archiveDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("archived %d files, want 3", len(entries))
	}
}

func TestReconcileAdoptsOrphanAndQuarantinesCorrupt(t *testing.T) {
	dir := t.TempDir()
	gen := seqID()
	store, _ := Open(dir, WithIDGen(gen))
	p := newTestProject(t)
	_ = p.Apply(project.Operation{Seq: 1, Kind: project.OpAddClip, ClipID: "c1", MediaID: "m1", Start: 0, End: 5})
	base, err := store.SaveBase(p)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a crash after snapshot data is fsynced but before manifest update:
	// write the next snapshot file, then roll the manifest back.
	_ = p.Apply(project.Operation{Seq: 2, Kind: project.OpMoveClip, ClipID: "c1", Track: 1, Position: 2})
	snap, err := buildSnapshot(store, p, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writeSnapshot(snap); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir, WithIDGen(gen))
	if err != nil {
		t.Fatal(err)
	}
	manifest, _ := reopened.Manifest()
	if manifest.Head == nil || manifest.Head.Seq != 2 {
		t.Fatalf("orphan not adopted: %+v", manifest)
	}

	// A corrupt orphan must be quarantined, not adopted.
	_ = p.Apply(project.Operation{Seq: 3, Kind: project.OpMoveClip, ClipID: "c1", Track: 2, Position: 3})
	bad, err := buildSnapshot(reopened, p, snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.writeSnapshot(bad); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, reopened.snapRel(bad)), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	reopened2, err := Open(dir, WithIDGen(gen))
	if err != nil {
		t.Fatal(err)
	}
	m2, _ := reopened2.Manifest()
	if m2.Head.Seq != 2 {
		t.Fatalf("corrupt orphan moved head to %d", m2.Head.Seq)
	}
	if _, err := os.Stat(filepath.Join(dir, archiveDir, filepath.Base(reopened.snapRel(bad)))); err != nil {
		t.Fatalf("corrupt file not quarantined: %v", err)
	}
}

func TestChecksumDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(dir, WithIDGen(seqID()))
	p := newTestProject(t)
	_ = p.Apply(project.Operation{Seq: 1, Kind: project.OpAddClip, ClipID: "c1", MediaID: "m1", Start: 0, End: 5})
	snap, err := store.SaveBase(p)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, store.snapRel(snap))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadSnapshot(store.snapRel(snap)); !errors.Is(err, ErrChecksum) {
		t.Fatalf("expected ErrChecksum, got %v", err)
	}
}

func buildSnapshot(store *Store, p *project.Project, prev *Snapshot) (*Snapshot, error) {
	manifest, err := store.readManifest()
	if err != nil {
		return nil, err
	}
	newOps := operationsAfter(p, manifest.Head.Seq)
	snap := &Snapshot{
		ID:         store.idGen(),
		ProjectID:  p.ID,
		Kind:       KindIncremental,
		Seq:        p.LastSeq(),
		BaseSeq:    manifest.Base.Seq,
		ParentID:   manifest.Base.ID,
		PrevID:     prev.ID,
		CreatedAt:  store.now().UTC().Format("2006-01-02T15:04:05Z"),
		StateHash:  p.StateHash(),
		Operations: newOps,
		Media:      mediaList(p),
	}
	return snap, nil
}

func seqID() func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("%032d", n)
	}
}

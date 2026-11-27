package recovery

import (
	"os"
	"path/filepath"
	"testing"

	"editorautosave/autosave"
	"editorautosave/project"
)

func buildChain(t *testing.T, dir string) (*autosave.Store, *project.Project, string) {
	t.Helper()
	mediaPath := filepath.Join(dir, "media.mp4")
	if err := os.WriteFile(mediaPath, []byte("media-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := autosave.Open(filepath.Join(dir, "recovery"), autosave.WithBaseEvery(10))
	if err != nil {
		t.Fatal(err)
	}
	p := project.New("edit-1")
	p.RegisterMedia(project.MediaRef{ID: "m1", Path: mediaPath, Size: 11})
	_ = p.Apply(project.Operation{Seq: 1, Kind: project.OpAddClip, ClipID: "c1", MediaID: "m1", Start: 0, End: 4})
	if _, err := store.SaveBase(p); err != nil {
		t.Fatal(err)
	}

	// Disk project is deliberately stale at seq 1.
	stale := p.Clone()
	diskPath := filepath.Join(dir, "edit.json")
	if err := project.Save(diskPath, stale); err != nil {
		t.Fatal(err)
	}

	_ = p.Apply(project.Operation{Seq: 2, Kind: project.OpMoveClip, ClipID: "c1", Track: 1, Position: 3})
	if _, err := store.AppendIncremental(p); err != nil {
		t.Fatal(err)
	}
	_ = p.Apply(project.Operation{Seq: 3, Kind: project.OpTrimClip, ClipID: "c1", Start: 1, End: 2})
	if _, err := store.AppendIncremental(p); err != nil {
		t.Fatal(err)
	}
	return store, p, diskPath
}

func TestInspectFindsNewerCandidate(t *testing.T) {
	dir := t.TempDir()
	store, live, diskPath := buildChain(t, dir)

	report, err := Inspect(store, diskPath, NewFileVerifier(false))
	if err != nil {
		t.Fatal(err)
	}
	if report.DiskProjectSeq != 1 {
		t.Fatalf("disk seq = %d, want 1", report.DiskProjectSeq)
	}
	if !report.HasNewerCandidate() || report.Best.Seq != 3 {
		t.Fatalf("best candidate wrong: %+v", report.Best)
	}
	if len(report.Candidates) != 3 || report.Candidates[0].Seq != 3 {
		t.Fatal("candidates must be sorted newest-first")
	}

	result, err := Restore(store, report.Best.ID, diskPath, filepath.Join(dir, "out"), NewFileVerifier(false))
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := project.Load(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.LastSeq() != 3 || recovered.StateHash() != live.StateHash() {
		t.Fatal("recovered project does not match live state")
	}
	if !result.OriginalKept {
		t.Fatal("original project must remain in place")
	}
	disk, _ := project.Load(diskPath)
	if disk.LastSeq() != 1 {
		t.Fatal("original on-disk project was modified")
	}
}

func TestCorruptHeadOffersNearestValidSnapshot(t *testing.T) {
	dir := t.TempDir()
	store, _, diskPath := buildChain(t, dir)
	manifest, _ := store.Manifest()

	if err := os.WriteFile(filepath.Join(store.Dir(), manifest.Head.File), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Inspect(store, diskPath, NewFileVerifier(false))
	if err != nil {
		t.Fatal(err)
	}
	if report.Break == nil || report.Break.Seq != 3 {
		t.Fatalf("expected break at seq 3, got %+v", report.Break)
	}
	if report.Best == nil || report.Best.Seq != 2 {
		t.Fatalf("expected nearest valid seq 2, got %+v", report.Best)
	}

	result, err := Restore(store, report.Best.ID, diskPath, filepath.Join(dir, "out"), NewFileVerifier(false))
	if err != nil {
		t.Fatal(err)
	}
	if result.RestoredSeq != 2 {
		t.Fatalf("restored seq = %d, want 2", result.RestoredSeq)
	}
}

func TestMissingMediaReportedButRecoveryProceeds(t *testing.T) {
	dir := t.TempDir()
	store, _, diskPath := buildChain(t, dir)
	mediaPath := filepath.Join(dir, "media.mp4")
	if err := os.Remove(mediaPath); err != nil {
		t.Fatal(err)
	}
	report, err := Inspect(store, diskPath, NewFileVerifier(false))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Best.MissingMedia) == 0 {
		t.Fatal("expected missing media warning on best candidate")
	}
	result, err := Restore(store, report.Best.ID, diskPath, filepath.Join(dir, "out"), NewFileVerifier(false))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MissingMedia) == 0 {
		t.Fatal("restore result must carry missing media warnings")
	}
}

func TestRestoreNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	store, _, diskPath := buildChain(t, dir)
	report, err := Inspect(store, diskPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	first, err := Restore(store, report.Best.ID, diskPath, out, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Restore(store, report.Best.ID, diskPath, out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.OutputPath == second.OutputPath {
		t.Fatal("second restore must choose a unique path")
	}
}

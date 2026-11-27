// Package demo runs an end-to-end autosave/recovery scenario on a temporary
// workspace, reproducing the "disk project older than recovery point" crash.
package demo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"editorautosave/autosave"
	"editorautosave/project"
	"editorautosave/recovery"
)

// Summary collects the facts the scenario asserts against.
type Summary struct {
	Root               string
	DiskProjectPath    string
	RecoveredFirst     string
	RecoveredFallback  string
	LastSuccessSeq     int
	DiskSeqBeforeCrash int
	HeadSeqBeforeCrash int
	BreakSnapshotID    string
	FallbackSeq        int
	MissingMedia       []string
	OriginalUntouched  bool
}

// Run builds the workspace, writes a base plus incrementals, simulates a
// crash with a stale on-disk project, then a corrupt-head crash, and performs
// fail-safe recoveries into separate files.
func Run(root string) (*Summary, error) {
	dirs := struct {
		media, project, recovery, recovered string
	}{
		media:     filepath.Join(root, "media"),
		project:   filepath.Join(root, "project"),
		recovery:  filepath.Join(root, "recovery"),
		recovered: filepath.Join(root, "recovered"),
	}
	for _, dir := range []string{dirs.media, dirs.project, dirs.recovery, dirs.recovered} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	mediaA, err := writeMedia(dirs.media, "clip-a.mp4", []byte("video-frame-data-A"))
	if err != nil {
		return nil, err
	}
	mediaB, err := writeMedia(dirs.media, "clip-b.mp4", []byte("video-frame-data-B"))
	if err != nil {
		return nil, err
	}

	counter := newIDCounter()
	store, err := autosave.Open(dirs.recovery,
		autosave.WithIDGen(counter.gen),
		autosave.WithBaseEvery(10),
	)
	if err != nil {
		return nil, err
	}

	edit := project.New("demo-edit")
	edit.RegisterMedia(mediaA)
	edit.RegisterMedia(mediaB)

	if err := edit.Apply(project.Operation{
		Seq: 1, Kind: project.OpAddClip, ClipID: "c1", MediaID: mediaA.ID,
		Track: 0, Start: 0, End: 5, Position: 0,
	}); err != nil {
		return nil, err
	}
	if _, err := store.Auto(edit); err != nil { // base snapshot
		return nil, err
	}

	if err := edit.Apply(project.Operation{
		Seq: 2, Kind: project.OpAddClip, ClipID: "c2", MediaID: mediaB.ID,
		Track: 0, Start: 0, End: 3, Position: 5,
	}); err != nil {
		return nil, err
	}
	if _, err := store.Auto(edit); err != nil { // incremental
		return nil, err
	}

	staleDisk := edit.Clone() // disk reflects state through seq 2
	diskPath := filepath.Join(dirs.project, "edit.json")
	if err := project.Save(diskPath, staleDisk); err != nil {
		return nil, err
	}

	if err := edit.Apply(project.Operation{
		Seq: 3, Kind: project.OpTrimClip, ClipID: "c1", Start: 1, End: 4,
	}); err != nil {
		return nil, err
	}
	if _, err := store.Auto(edit); err != nil { // incremental head at seq 3
		return nil, err
	}

	manifest, err := store.Manifest()
	if err != nil {
		return nil, err
	}
	summary := &Summary{
		Root:               root,
		DiskProjectPath:    diskPath,
		DiskSeqBeforeCrash: staleDisk.LastSeq(),
		HeadSeqBeforeCrash: manifest.Head.Seq,
		LastSuccessSeq:     manifest.LastSuccessSeq,
	}

	verifier := recovery.NewFileVerifier(true)
	report, err := recovery.Inspect(store, diskPath, verifier)
	if err != nil {
		return nil, err
	}
	if !report.HasNewerCandidate() {
		return nil, fmt.Errorf("expected newer recovery candidate over disk seq %d", report.DiskProjectSeq)
	}

	first, err := recovery.Restore(store, report.Best.ID, diskPath, dirs.recovered, verifier)
	if err != nil {
		return nil, err
	}
	summary.RecoveredFirst = first.OutputPath

	// Second crash: the newest snapshot is torn/corrupt on disk.
	headFile := filepath.Join(store.Dir(), manifest.Head.File)
	if err := os.WriteFile(headFile, []byte("{corrupt-payload"), 0o644); err != nil {
		return nil, err
	}
	// One media dependency disappears as well.
	if err := os.Remove(filepath.Join(dirs.media, "clip-b.mp4")); err != nil {
		return nil, err
	}

	report2, err := recovery.Inspect(store, diskPath, verifier)
	if err != nil {
		return nil, err
	}
	if report2.Break == nil {
		return nil, fmt.Errorf("expected chain break after corrupting head")
	}
	summary.BreakSnapshotID = report2.Break.SnapshotID

	// The nearest still-valid snapshot is offered for manual selection.
	var fallback *recovery.Candidate
	for i := range report2.Candidates {
		cand := &report2.Candidates[i]
		if cand.Reachable {
			fallback = cand
			break // candidates are sorted newest-first
		}
	}
	if fallback == nil {
		return nil, fmt.Errorf("no reachable fallback candidate after head corruption")
	}
	second, err := recovery.Restore(store, fallback.ID, diskPath, dirs.recovered, verifier)
	if err != nil {
		return nil, err
	}
	summary.FallbackSeq = second.RestoredSeq
	summary.RecoveredFallback = second.OutputPath
	summary.MissingMedia = second.MissingMedia

	diskAfter, err := project.Load(diskPath)
	if err != nil {
		return nil, err
	}
	summary.OriginalUntouched = diskAfter.LastSeq() == staleDisk.LastSeq()
	return summary, nil
}

func writeMedia(dir, name string, payload []byte) (project.MediaRef, error) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return project.MediaRef{}, err
	}
	sum := sha256.Sum256(payload)
	return project.MediaRef{
		ID:   "media-" + name,
		Path: path,
		Size: int64(len(payload)),
		Hash: hex.EncodeToString(sum[:]),
	}, nil
}

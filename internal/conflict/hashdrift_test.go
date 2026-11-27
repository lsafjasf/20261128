package conflict

import (
	"os"
	"path/filepath"
	"testing"

	"clipmerge/internal/manifest"
)

func TestAnalyzeDetectsOnDiskHashDrift(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join("day1", "c1.mov")
	if err := os.MkdirAll(filepath.Join(root, "day1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, mediaPath), []byte("actual bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Media: []manifest.Media{
			{UID: "U1", Path: mediaPath, HashAlgo: "sha256", Hash: "1111111111111111111111111111111111111111111111111111111111111111", ShootDay: "day1"},
		},
		Clips: []manifest.Clip{
			{ID: "C1", ShootDay: "day1", Bin: "B", ClipNo: "001", LockedUID: "U1", LockedHash: "1111111111111111111111111111111111111111111111111111111111111111"},
		},
	}

	rep := Analyze(m, root)
	foundDrift, foundBlocked := false, false
	for _, g := range rep.Groups {
		if g.Kind == KindHashDrift {
			foundDrift = true
		}
		if g.Kind == KindRollbackBlocked {
			foundBlocked = true
			if g.WrongLocks[0].ResolvedBy != "blocked" {
				t.Fatalf("content-swap rollback must be blocked: %+v", g.WrongLocks[0])
			}
		}
	}
	if !foundDrift {
		t.Fatal("HASH_DRIFT not detected for swapped file content")
	}
	if !foundBlocked {
		t.Fatal("clip bound to swapped content must require explicit blocked rollback")
	}
}

package autosave

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStaleTmpIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "snap-000099-deadbeef.json.tmp"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := store.Manifest()
	if len(m.Snapshots) != 0 {
		t.Fatalf("tmp file must not be adopted: %+v", m.Snapshots)
	}
	if _, err := os.Stat(filepath.Join(dir, "snap-000099-deadbeef.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("stale tmp file should be removed, stat err=%v", err)
	}
}

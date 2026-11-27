package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestColocationSeparationDetected(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/photo.CR2", "raw")
	writeFile(t, root, "a/photo.xmp", "meta")
	m, err := Scan(root, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Move only the sidecar into another directory.
	if err := os.MkdirAll(filepath.Join(root, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "a", "photo.xmp"),
		filepath.Join(root, "b", "photo.xmp")); err != nil {
		t.Fatal(err)
	}
	r, err := Verify(m, root, VerifyOptions{IgnoreExtras: true, ExpectColoc: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.OK || len(r.ColocationIssue) != 1 {
		t.Fatalf("expected one colocation issue: %+v", r)
	}
	// Without the colocation flag, content verification still passes.
	r2, err := Verify(m, root, VerifyOptions{IgnoreExtras: true})
	if err != nil || !r2.OK || len(r2.Rehomed) != 1 {
		t.Fatalf("content-only verify should pass: %+v err=%v", r2, err)
	}
}

func TestSymlinkIsSkippedAndReported(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "real/photo.jpg", "jpeg-bytes")
	if err := os.Symlink(filepath.Join(root, "real", "photo.jpg"),
		filepath.Join(root, "link.jpg")); err != nil {
		t.Skipf("symlink unsupported on platform: %v", err)
	}
	m, err := Scan(root, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if findIssue(m, "symlink_skipped") == nil {
		t.Fatal("expected symlink_skipped issue")
	}
	for _, e := range m.Entries {
		if e.Path == "link.jpg" {
			t.Fatal("symlink must not be archived as an entry")
		}
	}
}

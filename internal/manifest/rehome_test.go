package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDuplicateContentRehomedMultiset(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "old/a/dup.bin", "same-content")
	writeFile(t, root, "old/b/dup.bin", "same-content")
	writeFile(t, root, "old/c/unique.bin", "unique")
	m, err := Scan(root, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Re-arrange: both duplicates land in one new directory with new names.
	for _, dir := range []string{"new/pool", "new/misc"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	moves := map[string]string{
		"old/a/dup.bin":    "new/pool/copy1.bin",
		"old/b/dup.bin":    "new/pool/copy2.bin",
		"old/c/unique.bin": "new/misc/unique.bin",
	}
	for from, to := range moves {
		if err := os.Rename(filepath.Join(root, filepath.FromSlash(from)),
			filepath.Join(root, filepath.FromSlash(to))); err != nil {
			t.Fatal(err)
		}
	}

	r, err := Verify(m, root, VerifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK || len(r.Missing) != 0 || len(r.Extra) != 0 {
		t.Fatalf("all three entries should re-home 1:1, got %+v", r)
	}
	if len(r.Rehomed) != 3 {
		t.Fatalf("want 3 rehomed entries, got %+v", r.Rehomed)
	}
	used := map[string]bool{}
	for _, re := range r.Rehomed {
		if used[re.NewPath] {
			t.Fatalf("new path used twice: %s", re.NewPath)
		}
		used[re.NewPath] = true
	}
}

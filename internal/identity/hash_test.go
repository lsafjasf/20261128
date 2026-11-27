package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSHA256File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.bin")
	if err := os.WriteFile(path, []byte("clipmerge\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "cce5fd65fdc51cc5fbd9ab827a00be1043c55aa7e11f960589b89b1f78436324"
	if got != want {
		t.Fatalf("sha256 mismatch: got %s want %s", got, want)
	}

	if _, err := SHA256File(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeManifestForTest(path string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildTree(t *testing.T) string {
	root := t.TempDir()
	writeFile(t, root, "trip/day1/photo.CR2", "raw-bytes-A")
	writeFile(t, root, "trip/day1/photo.xmp", "xmp-A")
	writeFile(t, root, "trip/day1/clip.mp4", "video-A")
	writeFile(t, root, "trip/day1/clip.mp4.srt", "sub-A")
	writeFile(t, root, "trip/day2/photo.CR2", "raw-bytes-A") // duplicate of day1 raw
	writeFile(t, root, "trip/day2/photo.xmp", "xmp-B")
	writeFile(t, root, "trip/day2/lonely.xmp", "orphan")
	writeFile(t, root, "trip/notes.txt", "plain text")
	return root
}

func findIssue(m *Manifest, code string) *Issue {
	for i := range m.Issues {
		if m.Issues[i].Code == code {
			return &m.Issues[i]
		}
	}
	return nil
}

func findDuplicate(m *Manifest, hash string) *DuplicateGroup {
	for i := range m.Duplicates {
		if m.Duplicates[i].SHA256 == hash {
			return &m.Duplicates[i]
		}
	}
	return nil
}

func TestScanRelationshipsDuplicatesAndRelativePaths(t *testing.T) {
	root := buildTree(t)
	m, err := Scan(root, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Root != "." || m.PathStyle != "posix-relative" {
		t.Fatalf("manifest must stay relative: root=%q style=%q", m.Root, m.PathStyle)
	}
	for _, e := range m.Entries {
		if filepath.IsAbs(e.Path) {
			t.Fatalf("absolute path leaked into manifest: %s", e.Path)
		}
	}
	if len(m.Relationships) != 3 {
		t.Fatalf("want 3 sidecar relationships, got %d: %+v", len(m.Relationships), m.Relationships)
	}
	if findIssue(m, "orphan_sidecar") == nil {
		t.Fatal("expected orphan_sidecar issue for lonely.xmp")
	}

	var rawHash string
	for _, e := range m.Entries {
		if e.Path == "trip/day1/photo.CR2" {
			rawHash = e.SHA256
		}
	}
	if rawHash == "" {
		t.Fatal("missing raw entry")
	}
	dup := findDuplicate(m, rawHash)
	if dup == nil || len(dup.Paths) != 2 || dup.Wasted <= 0 {
		t.Fatalf("duplicate detection failed: %+v", dup)
	}
	if m.Budget == nil || !m.Budget.Over || m.Budget.Reclaimable != dup.Wasted {
		t.Fatalf("budget summary wrong: %+v", m.Budget)
	}

	// Relationship content hashes must match their entries.
	for _, r := range m.Relationships {
		if r.MediaHash == "" || r.SidecarHash == "" {
			t.Fatalf("relationship missing hashes: %+v", r)
		}
	}
}

func TestVerifyRehomedAfterReLayout(t *testing.T) {
	root := buildTree(t)
	m, err := Scan(root, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Re-arrange: move day1 media + sidecar into a new folder, identical bytes.
	oldDir := filepath.Join(root, "trip", "day1")
	newDir := filepath.Join(root, "archive", "day1")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"photo.CR2", "photo.xmp", "clip.mp4", "clip.mp4.srt"} {
		if err := os.Rename(filepath.Join(oldDir, name), filepath.Join(newDir, name)); err != nil {
			t.Fatal(err)
		}
	}

	r, err := Verify(m, root, VerifyOptions{IgnoreExtras: true})
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK {
		t.Fatalf("re-layout tolerant verify should pass: %+v", r)
	}
	if len(r.Rehomed) != 4 || len(r.Missing) != 0 {
		t.Fatalf("expected 4 rehomed, got %+v", r.Rehomed)
	}

	// Colocation check must still pass: media and xmp moved together.
	rc, err := Verify(m, root, VerifyOptions{IgnoreExtras: true, ExpectColoc: true})
	if err != nil || !rc.OK {
		t.Fatalf("colocation verify failed: %+v err=%v", rc, err)
	}

	// Strict mode must reject moved paths.
	rs, err := Verify(m, root, VerifyOptions{Strict: true, IgnoreExtras: true})
	if err != nil {
		t.Fatal(err)
	}
	if rs.OK || len(rs.Missing) != 4 {
		t.Fatalf("strict verify should fail with missing entries: %+v", rs)
	}
}

func TestVerifyMissingAndCorrupt(t *testing.T) {
	root := buildTree(t)
	m, err := Scan(root, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Delete one file and corrupt another.
	if err := os.Remove(filepath.Join(root, "trip", "day2", "lonely.xmp")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "trip/day1/clip.mp4", "tampered video bytes")

	r, err := Verify(m, root, VerifyOptions{IgnoreExtras: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.OK {
		t.Fatal("verify must fail with missing and corrupt files")
	}
	if len(r.Missing) != 1 || r.Missing[0] != "trip/day2/lonely.xmp" {
		t.Fatalf("missing detection failed: %+v", r.Missing)
	}
	if len(r.Corrupt) != 1 || r.Corrupt[0].Path != "trip/day1/clip.mp4" {
		t.Fatalf("corrupt detection failed: %+v", r.Corrupt)
	}
}

func TestExcludeAndRoundTrip(t *testing.T) {
	root := buildTree(t)
	m, err := Scan(root, 0, map[string]bool{"trip/notes.txt": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range m.Entries {
		if e.Path == "trip/notes.txt" {
			t.Fatal("excluded file still present in manifest")
		}
	}
	out := filepath.Join(t.TempDir(), "manifest.json")
	if err := writeManifestForTest(out, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(out)
	if err != nil || loaded.EntryCount != m.EntryCount {
		t.Fatalf("round trip failed: %v %+v", err, loaded)
	}
}

package renderpipe

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const sourceHashDomain = "renderpipe/source/v1"

// SourceFile describes one regular file in a canonical source snapshot.
type SourceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// SourceSnapshot is the timezone- and filesystem-metadata-independent view of
// a project. Files are ordered by their normalized relative path.
type SourceSnapshot struct {
	Hash  string       `json:"source_sha256"`
	Files []SourceFile `json:"files"`
}

// SourceOptions controls which project files participate in the source hash.
type SourceOptions struct {
	// Ignore contains paths relative to the project root. Ignored directories
	// are skipped without descending into them.
	Ignore []string
}

// SnapshotSource hashes all regular files below root. It deliberately does not
// include file modification times, ownership, permissions, or absolute paths.
func SnapshotSource(root string, opts SourceOptions) (SourceSnapshot, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("resolve project root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("stat project root: %w", err)
	}
	if !info.IsDir() {
		return SourceSnapshot{}, fmt.Errorf("project root %q is not a directory", root)
	}

	ignored, err := normalizeIgnores(opts.Ignore)
	if err != nil {
		return SourceSnapshot{}, err
	}

	var files []SourceFile
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("make %q relative: %w", path, err)
		}
		if rel == "." {
			return nil
		}
		rel = slashPath(rel)

		if entry.IsDir() {
			if isAlwaysIgnored(rel) || isIgnored(rel, ignored) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source contains unsupported symlink %q", rel)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("source contains unsupported non-regular file %q", rel)
		}
		if isAlwaysIgnored(rel) || isIgnored(rel, ignored) {
			return nil
		}

		digest, size, err := hashFile(path)
		if err != nil {
			return err
		}
		files = append(files, SourceFile{Path: rel, SHA256: digest, Size: size})
		return nil
	})
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("scan source: %w", err)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	fields := make([]string, 0, len(files)*3)
	for _, file := range files {
		fields = append(fields, file.Path, file.SHA256, fmt.Sprint(file.Size))
	}
	return SourceSnapshot{Hash: joinHash(sourceHashDomain, fields...), Files: files}, nil
}

func hashFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open source file %q: %w", path, err)
	}
	defer file.Close()

	h := sha256.New()
	size, err := io.Copy(h, file)
	if err != nil {
		return "", 0, fmt.Errorf("hash source file %q: %w", path, err)
	}
	return hexSum(h.Sum(nil)), size, nil
}

func normalizeIgnores(paths []string) (map[string]struct{}, error) {
	ignored := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		if filepath.IsAbs(path) {
			return nil, fmt.Errorf("ignore path %q must be relative", path)
		}
		clean := slashPath(filepath.Clean(path))
		if clean == "." {
			return nil, fmt.Errorf("ignore path %q would ignore the project root", path)
		}
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("ignore path %q escapes the project root", path)
		}
		ignored[clean] = struct{}{}
	}
	return ignored, nil
}

func isIgnored(path string, ignored map[string]struct{}) bool {
	for {
		if _, ok := ignored[path]; ok {
			return true
		}
		parent := slashPath(filepath.Dir(path))
		if parent == "." || parent == path {
			return false
		}
		path = parent
	}
}

func isAlwaysIgnored(path string) bool {
	for _, part := range strings.Split(path, "/") {
		switch part {
		case ".git", ".hg", ".svn", ".renderpipe":
			return true
		}
	}
	return false
}

func slashPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

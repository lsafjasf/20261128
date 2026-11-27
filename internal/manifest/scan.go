package manifest

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"lto-manifest/internal/classify"
	"lto-manifest/internal/hashfile"
)

const schemaVersion = 1

// Scan walks root and builds a complete inventory. budgetLimit of zero means
// no capacity check is requested. Excluded relative paths are skipped.
func Scan(root string, budgetLimit int64, excluded map[string]bool) (*Manifest, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("scan root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scan root is not a directory: %s", root)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	m := &Manifest{
		SchemaVersion: schemaVersion,
		GeneratedAt:   now,
		HashAlgorithm: "sha256",
		PathStyle:     "posix-relative",
		Root:          ".",
		Entries:       []Entry{},
		Relationships: []Relationship{},
		Duplicates:    []DuplicateGroup{},
		Issues:        []Issue{},
	}

	err = filepath.WalkDir(root, func(absPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			m.Issues = append(m.Issues, Issue{Code: "walk_error", Detail: walkErr.Error()})
			return nil
		}
		rel, err := safeRelative(root, absPath)
		if err != nil {
			m.Issues = append(m.Issues, Issue{Code: "unsafe_path", Detail: err.Error()})
			return nil
		}
		if rel == "." {
			return nil
		}
		if excluded != nil && excluded[rel] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			m.Issues = append(m.Issues, Issue{Code: "symlink_skipped", Path: rel,
				Detail: "symlinks are not archived; copy the target file instead"})
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			m.Issues = append(m.Issues, Issue{Code: "nonregular_skipped", Path: rel,
				Detail: "not a regular file"})
			return nil
		}

		fInfo, err := d.Info()
		if err != nil {
			m.Issues = append(m.Issues, Issue{Code: "stat_error", Path: rel, Detail: err.Error()})
			return nil
		}
		sum, err := hashfile.SHA256(absPath)
		if err != nil {
			m.Issues = append(m.Issues, Issue{Code: "hash_error", Path: rel, Detail: err.Error()})
			return nil
		}
		name := d.Name()
		ext := classify.Ext(name)
		m.Entries = append(m.Entries, Entry{
			Path:      rel,
			Class:     classify.Class(name),
			MediaKind: classify.MediaKind(name),
			MIMEType:  classify.MIME(name),
			Ext:       ext,
			Size:      fInfo.Size(),
			ModTime:   fInfo.ModTime().UTC().Format(time.RFC3339),
			SHA256:    sum,
		})
		m.TotalSize += fInfo.Size()
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(m.Entries, func(i, j int) bool { return m.Entries[i].Path < m.Entries[j].Path })
	m.EntryCount = len(m.Entries)

	buildRelationships(m)
	buildDuplicates(m)
	if budgetLimit > 0 {
		buildBudget(m, budgetLimit)
	}
	return m, nil
}

// safeRelative converts an OS path to a cleaned, POSIX-style path relative to
// root. It rejects absolute inputs and any path escaping the tree.
func safeRelative(root, target string) (string, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes scan root: %s", rel)
	}
	return filepath.ToSlash(rel), nil
}

// ToPOSIX normalizes an externally supplied relative path the same way.
func ToPOSIX(rel string) string {
	return path.Clean(filepath.ToSlash(rel))
}

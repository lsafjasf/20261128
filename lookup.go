package batchout

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manifest is committed provenance for one successful task attempt.
type Manifest struct {
	Provenance
	CommittedAt time.Time `json:"committed_at"`
	JournalID   string    `json:"journal_id"`
}

// FileProvenance links one final path to the task that produced it.
type FileProvenance struct {
	Manifest
	Output OutputRecord `json:"output"`
}

// Lookup returns the committed provenance for a final file. The path may be
// relative to the output directory or an absolute path beneath it.
func (m *Manager) Lookup(name string) (*FileProvenance, error) {
	rel, err := m.outputRelative(name)
	if err != nil {
		return nil, err
	}
	manifests, err := m.Manifests()
	if err != nil {
		return nil, err
	}
	for _, manifest := range manifests {
		for _, output := range manifest.Outputs {
			if output.Path == rel {
				return &FileProvenance{Manifest: manifest, Output: output}, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, rel)
}

// LookupDigest finds provenance by the final file's SHA-256. It is useful after
// an intentional rename outside the managed transaction flow.
func (m *Manager) LookupDigest(digest string) ([]FileProvenance, error) {
	digest = strings.ToLower(strings.TrimSpace(digest))
	if len(digest) != 64 {
		return nil, errors.New("batchout: SHA-256 digest must contain 64 hex characters")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return nil, errors.New("batchout: SHA-256 digest must contain 64 hex characters")
	}
	manifests, err := m.Manifests()
	if err != nil {
		return nil, err
	}
	var matches []FileProvenance
	for _, manifest := range manifests {
		for _, output := range manifest.Outputs {
			if output.SHA256 == digest {
				matches = append(matches, FileProvenance{Manifest: manifest, Output: output})
			}
		}
	}
	return matches, nil
}

// Manifests returns all immutable committed provenance records.
func (m *Manager) Manifests() ([]Manifest, error) {
	entries, err := os.ReadDir(m.statePath(stateRecords))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	manifests := make([]Manifest, 0, len(names))
	for _, name := range names {
		taskID := strings.TrimSuffix(name, ".json")
		rec, err := m.loadRecord(taskID)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, Manifest{
			Provenance:  rec.Provenance,
			CommittedAt: rec.CommittedAt,
			JournalID:   rec.JournalID,
		})
	}
	return manifests, nil
}

func (m *Manager) outputRelative(name string) (string, error) {
	if filepath.IsAbs(name) {
		rel, err := filepath.Rel(m.outputDir, filepath.Clean(name))
		if err != nil {
			return "", err
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("batchout: path is outside output directory: %s", name)
		}
		name = rel
	}
	return cleanRelative(filepath.ToSlash(name))
}

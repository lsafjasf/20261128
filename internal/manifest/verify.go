package manifest

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"lto-manifest/internal/hashfile"
)

// VerifyOptions controls post-re-layout verification behavior.
type VerifyOptions struct {
	Strict       bool // require every manifest path to stay in place
	ExpectColoc  bool // require each sidecar to stay next to its media
	IgnoreExtras bool // do not fail on files absent from the manifest
}

// VerifyResult is the machine-readable verification report.
type VerifyResult struct {
	OK              bool      `json:"ok"`
	Strict          bool      `json:"strict"`
	Root            string    `json:"root"`
	Intact          []string  `json:"intact"`
	Rehomed         []Rehomed `json:"rehomed"`
	Missing         []string  `json:"missing"`
	Corrupt         []Corrupt `json:"corrupt"`
	Extra           []string  `json:"extra"`
	ColocationIssue []Issue   `json:"colocationIssues,omitempty"`
	Issues          []Issue   `json:"issues"`
}

// Rehomed records a content-identical file found at a new relative path.
type Rehomed struct {
	OldPath string `json:"oldPath"`
	NewPath string `json:"newPath"`
	SHA256  string `json:"sha256"`
}

// Corrupt records a present file whose content hash no longer matches.
type Corrupt struct {
	Path        string `json:"path"`
	ExpectedSHA string `json:"expectedSha256"`
	ActualSHA   string `json:"actualSha256"`
}

// Load reads a manifest previously written with encoding/json.
func Load(manifestPath string) (*Manifest, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.HashAlgorithm != "sha256" {
		return nil, fmt.Errorf("unsupported hash algorithm %q", m.HashAlgorithm)
	}
	return &m, nil
}

// Verify re-hashes the current tree at root against m.
func Verify(m *Manifest, root string, opts VerifyOptions) (*VerifyResult, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("verify root: %w", err)
	}
	result := &VerifyResult{
		Strict: opts.Strict,
		Root:   ".",
		Intact: []string{}, Rehomed: []Rehomed{}, Missing: []string{},
		Corrupt: []Corrupt{}, Extra: []string{}, Issues: []Issue{},
	}

	current := map[string]string{} // current relative path -> hash
	err := filepath.WalkDir(root, func(absPath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			result.Issues = append(result.Issues, Issue{Code: "walk_error", Detail: walkErr.Error()})
			return nil
		}
		rel, err := safeRelative(root, absPath)
		if err != nil {
			result.Issues = append(result.Issues, Issue{Code: "unsafe_path", Detail: err.Error()})
			return nil
		}
		if rel == "." || d.IsDir() || d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		sum, err := hashfile.SHA256(absPath)
		if err != nil {
			result.Issues = append(result.Issues, Issue{Code: "hash_error", Path: rel, Detail: err.Error()})
			return nil
		}
		current[rel] = sum
		return nil
	})
	if err != nil {
		return nil, err
	}

	// hash -> sorted current paths holding that content
	byHash := map[string][]string{}
	for rel, sum := range current {
		byHash[sum] = append(byHash[sum], rel)
	}
	for _, homes := range byHash {
		sort.Strings(homes)
	}

	// Reserve homes per manifest entry path. Relationship members are handled
	// first and prefer colocated media/sidecar homes, which keeps duplicated
	// content matched one-to-one and preserves pairings through re-layouts.
	reserved, reservedUsed := reserveHomes(m, byHash)
	cursor := map[string]int{}
	consume := func(hash string) (string, bool) {
		homes := byHash[hash]
		for cursor[hash] < len(homes) {
			home := homes[cursor[hash]]
			cursor[hash]++
			if !reservedUsed[home] {
				return home, true
			}
		}
		return "", false
	}

	seen := map[string]bool{}
	for _, e := range m.Entries {
		rel := ToPOSIX(e.Path)
		switch sum, present := current[rel]; {
		case present && sum == e.SHA256:
			result.Intact = append(result.Intact, rel)
			seen[rel] = true
		case present:
			result.Corrupt = append(result.Corrupt, Corrupt{
				Path: rel, ExpectedSHA: e.SHA256, ActualSHA: sum})
			seen[rel] = true
		default:
			if opts.Strict {
				result.Missing = append(result.Missing, rel)
				continue
			}
			if home, ok := reserved[rel]; ok {
				result.Rehomed = append(result.Rehomed, Rehomed{
					OldPath: rel, NewPath: home, SHA256: e.SHA256})
				seen[home] = true
				continue
			}
			home, ok := consume(e.SHA256)
			if !ok {
				result.Missing = append(result.Missing, rel)
				continue
			}
			result.Rehomed = append(result.Rehomed, Rehomed{
				OldPath: rel, NewPath: home, SHA256: e.SHA256})
			seen[home] = true
		}
	}

	for rel := range current {
		if !seen[rel] {
			result.Extra = append(result.Extra, rel)
		}
	}
	if opts.ExpectColoc {
		checkColocation(m, result)
	}
	sortResult(result)

	result.OK = len(result.Missing) == 0 && len(result.Corrupt) == 0 &&
		len(result.Issues) == 0 &&
		(opts.IgnoreExtras || len(result.Extra) == 0) &&
		(!opts.Strict || len(result.Rehomed) == 0) &&
		len(result.ColocationIssue) == 0
	return result, nil
}

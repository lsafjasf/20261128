// Package recovery inspects an autosave chain and restores a snapshot into a
// new file, never overwriting the original on-disk project.
package recovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"editorautosave/autosave"
	"editorautosave/project"
)

// Break describes where and why the recovery chain stopped being valid.
type Break struct {
	SnapshotID string
	Seq        int
	Reason     string
}

// Candidate is one manually selectable recovery point.
type Candidate struct {
	ID           string
	Seq          int
	Kind         string
	CreatedAt    string
	Reachable    bool
	StateHash    string
	MissingMedia []string
	Error        string
}

// Report is the result of inspecting a recovery directory.
type Report struct {
	ProjectID       string
	DiskProjectSeq  int
	DiskProjectPath string
	LastSuccessSeq  int
	LastSuccessAt   string
	Candidates      []Candidate
	Best            *Candidate
	Break           *Break
}

// HasNewerCandidate reports whether a reachable snapshot is newer than the
// on-disk project, which is exactly the situation where work was at risk.
func (r *Report) HasNewerCandidate() bool {
	return r.Best != nil && r.Best.Seq > r.DiskProjectSeq
}

// MediaVerifier checks that a media dependency is still available.
type MediaVerifier interface {
	Verify(ref project.MediaRef) error
}

// FileVerifier stat-checks media files on disk and optionally verifies size
// and sha256 hash when those fields were recorded.
type FileVerifier struct {
	Root      string
	CheckHash bool
}

// NewFileVerifier returns the default filesystem-backed verifier.
func NewFileVerifier(checkHash bool) *FileVerifier {
	return &FileVerifier{CheckHash: checkHash}
}

// Verify implements MediaVerifier.
func (v *FileVerifier) Verify(ref project.MediaRef) error {
	path := ref.Path
	if !filepath.IsAbs(path) && v.Root != "" {
		path = filepath.Join(v.Root, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("media %s unavailable: %w", ref.ID, err)
	}
	if ref.Size > 0 && info.Size() != ref.Size {
		return fmt.Errorf("media %s size mismatch: want %d, got %d", ref.ID, ref.Size, info.Size())
	}
	if v.CheckHash && ref.Hash != "" {
		got, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("media %s hash: %w", ref.ID, err)
		}
		if got != ref.Hash {
			return fmt.Errorf("media %s content hash mismatch", ref.ID)
		}
	}
	return nil
}

// Inspect walks every snapshot in the manifest, replays as much of the chain
// as possible and lists every usable point so the user can choose manually.
func Inspect(store *autosave.Store, diskProjectPath string, verifier MediaVerifier) (*Report, error) {
	manifest, err := store.Manifest()
	if err != nil {
		return nil, err
	}
	report := &Report{
		ProjectID:       manifest.ProjectID,
		DiskProjectPath: diskProjectPath,
		LastSuccessSeq:  manifest.LastSuccessSeq,
		LastSuccessAt:   manifest.LastSuccessAt,
	}

	if diskProjectPath != "" {
		if disk, loadErr := project.Load(diskProjectPath); loadErr == nil {
			report.DiskProjectSeq = disk.LastSeq()
		}
	}

	var chain []autosave.Pointer
	for _, ptr := range manifest.Snapshots {
		chain = append(chain, ptr)
		cand := Candidate{ID: ptr.ID, Seq: ptr.Seq, Kind: ptr.Kind, Reachable: true}
		reachable, reason := verifyLink(store, &manifest, chain)
		if !reachable {
			cand.Reachable = false
			cand.Error = reason
			if report.Break == nil {
				report.Break = &Break{SnapshotID: ptr.ID, Seq: ptr.Seq, Reason: reason}
			}
		} else {
			state, replayErr := store.Replay(chain)
			if replayErr != nil {
				cand.Reachable = false
				cand.Error = replayErr.Error()
				if report.Break == nil {
					report.Break = &Break{SnapshotID: ptr.ID, Seq: ptr.Seq, Reason: replayErr.Error()}
				}
			} else {
				cand.StateHash = state.StateHash()
				cand.MissingMedia = checkMedia(state, verifier)
				if snap, readErr := store.ReadSnapshot(ptr.File); readErr == nil {
					cand.CreatedAt = snap.CreatedAt
				}
			}
		}
		report.Candidates = append(report.Candidates, cand)
	}

	for i := range report.Candidates {
		cand := &report.Candidates[i]
		if !cand.Reachable {
			continue
		}
		if report.Best == nil || cand.Seq > report.Best.Seq {
			best := *cand
			report.Best = &best
		}
	}
	sort.SliceStable(report.Candidates, func(i, j int) bool {
		return report.Candidates[i].Seq > report.Candidates[j].Seq
	})
	return report, nil
}

// verifyLink confirms structural integrity of the chain prefix up to chain.
func verifyLink(store *autosave.Store, manifest *autosave.Manifest, chain []autosave.Pointer) (bool, string) {
	known := map[string]autosave.Pointer{}
	for _, ptr := range manifest.Snapshots {
		known[ptr.ID] = ptr
	}
	for i, ptr := range chain {
		if _, ok := known[ptr.ID]; !ok {
			return false, "snapshot not registered in manifest"
		}
		snap, err := store.ReadSnapshot(ptr.File)
		if err != nil {
			return false, err.Error()
		}
		if i == 0 {
			if snap.Kind != autosave.KindBase {
				return false, "chain root is not a base snapshot"
			}
			continue
		}
		prev := chain[i-1]
		if snap.Kind != autosave.KindIncremental || snap.PrevID != prev.ID {
			return false, "incremental snapshot is not linked to its predecessor"
		}
		if snap.Seq <= prev.Seq {
			return false, "snapshot sequence does not advance"
		}
	}
	return true, ""
}

func checkMedia(state *project.Project, verifier MediaVerifier) []string {
	if verifier == nil {
		return nil
	}
	var missing []string
	for _, ref := range state.Media {
		if err := verifier.Verify(*ref); err != nil {
			missing = append(missing, err.Error())
		}
	}
	sort.Strings(missing)
	return missing
}

// Result describes where a restored project was written.
type Result struct {
	OutputPath   string
	OriginalPath string
	OriginalKept bool
	RestoredSeq  int
	StateHash    string
	MissingMedia []string
}

// Restore replays the chain up to candidateID and writes the recovered
// project to a new file. The original on-disk project is never modified or
// deleted; the caller opens the output after reviewing media warnings.
func Restore(store *autosave.Store, candidateID, diskProjectPath, outDir string, verifier MediaVerifier) (*Result, error) {
	if candidateID == "" {
		return nil, errors.New("candidate id is required")
	}
	manifest, err := store.Manifest()
	if err != nil {
		return nil, err
	}
	chain, err := chainUpTo(manifest.Snapshots, candidateID)
	if err != nil {
		return nil, err
	}
	state, err := store.Replay(chain)
	if err != nil {
		return nil, fmt.Errorf("restore replay failed: %w", err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	output := uniquePath(filepath.Join(outDir, recoveredName(diskProjectPath, candidateID)))
	if err := project.Save(output, state); err != nil {
		return nil, err
	}

	result := &Result{
		OutputPath:   output,
		OriginalPath: diskProjectPath,
		OriginalKept: diskProjectPath != "" && fileExists(diskProjectPath),
		RestoredSeq:  state.LastSeq(),
		StateHash:    state.StateHash(),
		MissingMedia: checkMedia(state, verifier),
	}
	return result, nil
}

func chainUpTo(pointers []autosave.Pointer, candidateID string) ([]autosave.Pointer, error) {
	var chain []autosave.Pointer
	for _, ptr := range pointers {
		chain = append(chain, ptr)
		if ptr.ID == candidateID {
			return chain, nil
		}
	}
	return nil, fmt.Errorf("candidate %s not found in recovery chain", candidateID)
}

func recoveredName(original, candidateID string) string {
	base := "project"
	if original != "" {
		base = strings.TrimSuffix(filepath.Base(original), filepath.Ext(original))
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	return fmt.Sprintf("%s.recovered-%s-%s.json", base, stamp, candidateID[:8])
}

func uniquePath(path string) string {
	if !fileExists(path) {
		return path
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.%d%s", stem, i, ext)
		if !fileExists(candidate) {
			return candidate
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

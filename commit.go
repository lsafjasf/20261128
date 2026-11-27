package batchout

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func (m *Manager) commitJournal(j *journal) (*Result, error) {
	j.State = StateCommitting
	if err := m.saveJournal(j); err != nil {
		return nil, fmt.Errorf("batchout: persist commit intent: %w", err)
	}
	return m.finishCommitting(j)
}

func (m *Manager) finishCommitting(j *journal) (*Result, error) {
	var existing *record
	if loaded, err := m.loadRecord(j.TaskID); err == nil {
		existing = loaded
		if loaded.AttemptID == j.AttemptID {
			// A crash may have happened after the record was written but
			// before every output was visible. Re-check all outputs below.
		} else {
			m.abortJournal(j, "superseded by committed attempt "+loaded.AttemptID)
			m.removeStage(j)
			return resultFromRecord(loaded), nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	stageDir := m.stagePath(j.TaskID, j.AttemptID)
	for i := range j.Outputs {
		out := &j.Outputs[i]
		source := filepath.Join(stageDir, nativeRel(out.Name))
		if err := m.placeOutput(j, out, source); err != nil {
			return nil, err
		}
	}

	rec := existing
	if rec == nil {
		rec = &record{
			Provenance:  j.Provenance,
			CommittedAt: time.Now().UTC(),
			JournalID:   filepath.Base(m.journalPath(j.TaskID, j.AttemptID)),
		}
	}
	rec.Provenance = j.Provenance
	rec.Outputs = append([]OutputRecord(nil), j.Outputs...)
	if err := writeJSONAtomic(m.recordPath(j.TaskID), rec, 0o644); err != nil {
		return nil, fmt.Errorf("batchout: write provenance: %w", err)
	}
	return m.finalizeCommitted(j, rec), nil
}

func (m *Manager) finalizeCommitted(j *journal, rec *record) *Result {
	j.State = StateCommitted
	j.UpdatedAt = time.Now().UTC()
	_ = m.saveJournal(j)
	m.removeStage(j)
	return resultFromRecord(rec)
}

func (m *Manager) placeOutput(j *journal, out *OutputRecord, source string) error {
	sourceInfo, sourceErr := os.Lstat(source)
	sourceExists := sourceErr == nil
	switch {
	case sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist):
		return fmt.Errorf("batchout: inspect staged output %s: %w", out.Name, sourceErr)
	case sourceExists && !sourceInfo.Mode().IsRegular():
		return fmt.Errorf("batchout: staged output %s is not a regular file", out.Name)
	case sourceExists && sourceInfo.Size() != out.Size:
		return fmt.Errorf("batchout: staged output %s changed after snapshot", out.Name)
	}

	for _, candidate := range targetCandidates(out.Name, j.TaskID) {
		target, err := joinBelow(m.outputDir, candidate)
		if err != nil {
			return err
		}
		parentRel := filepath.Dir(nativeRel(candidate))
		if err := ensureDirPath(m.outputDir, parentRel); err != nil {
			return err
		}
		exists, err := existsNoFollow(target)
		if err != nil {
			return err
		}
		if exists {
			sameFile := false
			if sourceExists {
				targetInfo, err := os.Lstat(target)
				if err != nil {
					return err
				}
				sameFile = os.SameFile(sourceInfo, targetInfo)
			}
			if sameFile {
				if !fileMatches(target, out.Size, out.SHA256) {
					return fmt.Errorf("batchout: committed target %s changed after snapshot", candidate)
				}
			}
			if sameFile || (!sourceExists && fileMatches(target, out.Size, out.SHA256)) {
				if out.Path != candidate {
					out.Path = candidate
					if err := m.saveJournal(j); err != nil {
						return err
					}
				}
				return nil
			}
			continue
		}

		if out.Path != candidate {
			out.Path = candidate
			if err := m.saveJournal(j); err != nil {
				return err
			}
		}
		err = os.Link(source, target)
		if err == nil {
			return syncDir(filepath.Dir(target))
		}
		if errors.Is(err, fs.ErrExist) || errors.Is(err, syscall.EEXIST) {
			continue
		}
		return fmt.Errorf("batchout: link %s: %w", candidate, err)
	}
	return fmt.Errorf("batchout: no free conflict name for %q", out.Name)
}

func fileMatches(path string, size int64, digest string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return false
	}
	actualSize, actualDigest, err := fileDigest(path)
	return err == nil && actualSize == size && actualDigest == digest
}

package batchout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const recoveryLockWait = 10 * time.Second

// Recover scans all transaction journals. Staging/prepared transactions are
// rolled back; committing transactions are completed idempotently; committed
// records are retained. Files with no journal are never touched.
func (m *Manager) Recover(ctx context.Context) (RecoveryReport, error) {
	lock, err := acquireNamedLock(ctx, m.lockPath("_recovery_"), lockOwner{
		TaskID:    "_recovery_",
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
	}, recoveryLockWait)
	if err != nil {
		return RecoveryReport{}, err
	}
	defer lock.Close()

	entries, err := os.ReadDir(m.statePath(stateJournals))
	if err != nil {
		return RecoveryReport{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	report := RecoveryReport{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		journalPath := m.statePath(stateJournals, entry.Name())
		j, err := m.readJournal(journalPath)
		if err != nil {
			report.Entries = append(report.Entries, RecoveryEntry{
				Journal: entry.Name(),
				Err:     err,
			})
			continue
		}
		result := m.recoverOne(ctx, journalPath, j)
		report.Entries = append(report.Entries, result)
	}
	return report, report.Err()
}

func (m *Manager) recoverJournalsLocked(ctx context.Context, taskID string) error {
	matches, err := filepath.Glob(m.statePath(stateJournals, taskID+"-*.json"))
	if err != nil {
		return err
	}
	var errs []error
	for _, journalPath := range matches {
		j, err := m.readJournal(journalPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(journalPath), err))
			continue
		}
		result := m.recoverOneLocked(journalPath, j)
		if result.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(journalPath), result.Err))
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) recoverOne(ctx context.Context, journalPath string, j *journal) RecoveryEntry {
	lock, ok, err := tryLockFile(m.lockPath(j.TaskID), lockOwner{
		TaskID:    j.TaskID,
		AttemptID: "recovery",
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		return RecoveryEntry{Journal: filepath.Base(journalPath), TaskID: j.TaskID, State: j.State, Err: err}
	}
	if !ok {
		return RecoveryEntry{
			Journal: filepath.Base(journalPath),
			TaskID:  j.TaskID,
			State:   j.State,
			Action:  RecoveredSkipped,
		}
	}
	defer lock.Close()

	// The journal may have advanced between the scan and lock acquisition.
	latest, err := m.readJournal(journalPath)
	if err != nil {
		return RecoveryEntry{Journal: filepath.Base(journalPath), TaskID: j.TaskID, State: j.State, Err: err}
	}
	return m.recoverOneLocked(journalPath, latest)
}

func (m *Manager) recoverOneLocked(journalPath string, j *journal) RecoveryEntry {
	entry := RecoveryEntry{Journal: filepath.Base(journalPath), TaskID: j.TaskID, State: j.State}
	switch j.State {
	case StateAborted:
		m.removeStage(j)
		entry.Action = RecoveredCleaned
	case StateStaging, StatePrepared:
		m.abortJournal(j, "recovered before commit")
		m.removeStage(j)
		entry.Action = RecoveredRolledBack
	case StateCommitting:
		_, err := m.finishCommitting(j)
		if err != nil {
			entry.Err = err
			return entry
		}
		entry.Action = RecoveredCommitted
	case StateCommitted:
		if _, err := m.loadRecord(j.TaskID); errors.Is(err, os.ErrNotExist) {
			if _, err := m.finishCommitting(j); err != nil {
				entry.Err = err
				return entry
			}
			entry.Action = RecoveredCommitted
			return entry
		} else if err != nil {
			entry.Err = err
			return entry
		}
		m.removeStage(j)
		entry.Action = RecoveredCleaned
	default:
		entry.Err = fmt.Errorf("batchout: unknown journal state %q", j.State)
	}
	return entry
}

func (m *Manager) readJournal(path string) (*journal, error) {
	var j journal
	if err := readJSON(path, &j); err != nil {
		return nil, err
	}
	if err := validateJournal(&j); err != nil {
		return nil, err
	}
	if filepath.Base(m.journalPath(j.TaskID, j.AttemptID)) != filepath.Base(path) {
		return nil, errors.New("batchout: journal filename does not match its identity")
	}
	return &j, nil
}

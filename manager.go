package batchout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Producer writes one or more regular files beneath stageDir. It must not
// assume that anything it writes is visible in the output directory.
type Producer func(ctx context.Context, stageDir string) error

type options struct {
	lockWait    time.Duration
	autoRecover bool
}

// Option customizes a Manager.
type Option func(*options)

// WithLockWait makes Run wait up to d for a held task lock. The default is to
// fail immediately with ErrTaskLocked.
func WithLockWait(d time.Duration) Option {
	return func(opts *options) {
		if d > 0 {
			opts.lockWait = d
		}
	}
}

// WithoutAutoRecover disables the recovery pass performed by Open. It is useful
// for maintenance tools that want to inspect the RecoveryReport themselves.
func WithoutAutoRecover() Option {
	return func(opts *options) {
		opts.autoRecover = false
	}
}

// Manager owns one output directory and its internal .batch transaction state.
type Manager struct {
	outputDir string
	opts      options
}

// Open prepares an output directory and, by default, repairs interrupted
// transactions. Recovery never touches a task whose OS lock is still held.
func Open(outputDir string, opts ...Option) (*Manager, error) {
	if outputDir == "" {
		return nil, errors.New("batchout: output directory is required")
	}
	root, err := filepath.Abs(outputDir)
	if err != nil {
		return nil, err
	}
	cfg := options{autoRecover: true}
	for _, opt := range opts {
		opt(&cfg)
	}
	m := &Manager{outputDir: root, opts: cfg}
	if err := m.ensureLayout(); err != nil {
		return nil, err
	}
	if cfg.autoRecover {
		report, err := m.Recover(context.Background())
		if err != nil {
			return nil, fmt.Errorf("batchout: recovery failed: %w", err)
		}
		_ = report
	}
	return m, nil
}

// OutputDir returns the absolute output directory.
func (m *Manager) OutputDir() string {
	return m.outputDir
}

func (m *Manager) statePath(parts ...string) string {
	all := append([]string{m.outputDir, stateRoot}, parts...)
	return filepath.Join(all...)
}

func (m *Manager) ensureLayout() error {
	if err := os.MkdirAll(m.outputDir, 0o755); err != nil {
		return err
	}
	for _, dir := range []string{
		stateRoot,
		filepath.Join(stateRoot, stateJournals),
		filepath.Join(stateRoot, stateLockDir),
		filepath.Join(stateRoot, stateTmp),
		filepath.Join(stateRoot, stateRecords),
	} {
		if err := ensureDirPath(m.outputDir, filepath.ToSlash(dir)); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) journalPath(taskID, attemptID string) string {
	return m.statePath(stateJournals, taskID+"-"+attemptID+".json")
}

func (m *Manager) recordPath(taskID string) string {
	return m.statePath(stateRecords, taskID+".json")
}

func (m *Manager) stagePath(taskID, attemptID string) string {
	return m.statePath(stateTmp, taskID, attemptID)
}

func (m *Manager) lockPath(taskID string) string {
	return m.statePath(stateLockDir, taskID+".lock")
}

func (m *Manager) acquireTaskLock(ctx context.Context, taskID, attemptID string) (*taskLock, error) {
	return acquireNamedLock(ctx, m.lockPath(taskID), lockOwner{
		TaskID:    taskID,
		AttemptID: attemptID,
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
	}, m.opts.lockWait)
}

func acquireNamedLock(ctx context.Context, path string, owner lockOwner, wait time.Duration) (*taskLock, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	for {
		lock, ok, err := tryLockFile(path, owner)
		if err != nil {
			return nil, err
		}
		if ok {
			return lock, nil
		}
		if wait <= 0 || time.Since(start) >= wait {
			return nil, fmt.Errorf("%w: %s", ErrTaskLocked, owner.TaskID)
		}
		delay := 25 * time.Millisecond
		remaining := wait - time.Since(start)
		if remaining < delay {
			delay = remaining
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// Run executes a task under its task lock. Outputs are staged, fingerprinted,
// journaled, and only then linked into the output tree without overwriting any
// existing path.
func (m *Manager) Run(ctx context.Context, spec TaskSpec, produce Producer) (*Result, error) {
	if produce == nil {
		return nil, errors.New("batchout: producer is required")
	}
	taskID, inputs, paramsDigest, params, err := fingerprint(spec)
	if err != nil {
		return nil, err
	}
	attemptID, err := newAttemptID()
	if err != nil {
		return nil, err
	}
	lock, err := m.acquireTaskLock(ctx, taskID, attemptID)
	if err != nil {
		return nil, err
	}
	defer lock.Close()

	if err := m.recoverJournalsLocked(ctx, taskID); err != nil {
		return nil, err
	}
	if existing, err := m.loadRecord(taskID); err == nil {
		return resultFromRecord(existing), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	now := time.Now().UTC()
	j := &journal{
		Provenance: Provenance{
			Version:      Version,
			TaskID:       taskID,
			AttemptID:    attemptID,
			Recipe:       spec.Recipe,
			Inputs:       inputs,
			Params:       params,
			ParamsDigest: paramsDigest,
		},
		State:     StateStaging,
		CreatedAt: now,
		UpdatedAt: now,
		StageDir:  filepath.ToSlash(filepath.Join(stateRoot, stateTmp, taskID, attemptID)),
	}
	if err := m.saveJournal(j); err != nil {
		return nil, err
	}

	stageDir := m.stagePath(taskID, attemptID)
	stageRel := filepath.ToSlash(filepath.Join(stateRoot, stateTmp, taskID, attemptID))
	if err := ensureDirPath(m.outputDir, stageRel); err != nil {
		m.abortJournal(j, "create staging directory: "+err.Error())
		return nil, err
	}
	if err := produce(ctx, stageDir); err != nil {
		m.abortJournal(j, "producer: "+err.Error())
		m.removeStage(j)
		return nil, fmt.Errorf("batchout: producer failed: %w", err)
	}

	outputs, err := snapshotOutputs(stageDir)
	if err != nil {
		m.abortJournal(j, "snapshot outputs: "+err.Error())
		m.removeStage(j)
		return nil, err
	}
	if len(outputs) == 0 {
		m.abortJournal(j, ErrNoOutput.Error())
		m.removeStage(j)
		return nil, ErrNoOutput
	}
	for i := range outputs {
		outputs[i].Path = outputs[i].Name
	}
	j.Outputs = outputs
	j.State = StatePrepared
	j.UpdatedAt = time.Now().UTC()
	if err := m.saveJournal(j); err != nil {
		m.abortJournal(j, "save prepared journal: "+err.Error())
		m.removeStage(j)
		return nil, err
	}

	result, err := m.commitJournal(j)
	if err != nil {
		// The journal remains in committing state. A later recovery pass can
		// finish it; never delete staged data on an ambiguous commit failure.
		return nil, err
	}
	return result, nil
}

func newAttemptID() (string, error) {
	suffix, err := randomID()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d-%s", time.Now().UTC().UnixNano(), suffix), nil
}

func (m *Manager) saveJournal(j *journal) error {
	j.UpdatedAt = time.Now().UTC()
	path := m.journalPath(j.TaskID, j.AttemptID)
	return writeJSONAtomic(path, j, 0o644)
}

func (m *Manager) abortJournal(j *journal, reason string) {
	j.State = StateAborted
	j.Error = reason
	_ = m.saveJournal(j)
}

func (m *Manager) removeStage(j *journal) {
	stage := m.stagePath(j.TaskID, j.AttemptID)
	_ = os.RemoveAll(stage)
	_ = os.Remove(filepath.Dir(stage))
}

func (m *Manager) loadRecord(taskID string) (*record, error) {
	if err := validateSHA256("task id", taskID); err != nil {
		return nil, fmt.Errorf("batchout: invalid task id: %w", err)
	}
	var rec record
	if err := readJSON(m.recordPath(taskID), &rec); err != nil {
		return nil, err
	}
	if rec.Version != Version || rec.TaskID != taskID {
		return nil, fmt.Errorf("batchout: invalid provenance record for task %s", taskID)
	}
	if err := validateProvenance(&rec.Provenance); err != nil {
		return nil, fmt.Errorf("batchout: invalid provenance record for task %s: %w", taskID, err)
	}
	return &rec, nil
}

func resultFromRecord(rec *record) *Result {
	outputs := append([]OutputRecord(nil), rec.Outputs...)
	return &Result{
		TaskID:     rec.TaskID,
		AttemptID:  rec.AttemptID,
		Outputs:    outputs,
		Provenance: rec.Provenance,
	}
}

package batchout

import (
	"errors"
	"fmt"
	"time"
)

const (
	// Version is the on-disk journal and provenance schema version.
	Version = 1

	stateRoot     = ".batch"
	stateJournals = "journals"
	stateLockDir  = "locks"
	stateTmp      = "tmp"
	stateRecords  = "records"
)

var (
	// ErrTaskLocked means another process or goroutine owns the task lock.
	ErrTaskLocked = errors.New("batchout: task is already running")
	// ErrNoOutput means the producer did not create a regular output file.
	ErrNoOutput = errors.New("batchout: producer created no output files")
	// ErrNotFound means no committed provenance record matches the file.
	ErrNotFound = errors.New("batchout: provenance not found")
)

// Input identifies one producer input. Name is the stable logical name used in
// fingerprints; Path is read only to calculate the content digest.
type Input struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// TaskSpec describes the identity of a batch task. Inputs and Params are part
// of the task fingerprint. Recipe can carry a pipeline or tool version so that
// changing the producer logic also changes the task identity.
type TaskSpec struct {
	Inputs []Input           `json:"inputs"`
	Params map[string]string `json:"params,omitempty"`
	Recipe string            `json:"recipe,omitempty"`
}

// InputRecord is the immutable input snapshot stored in task provenance.
type InputRecord struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// OutputRecord describes one committed file.
type OutputRecord struct {
	// Name is the relative path requested by the producer in the staging area.
	Name string `json:"name"`
	// Path is the actual relative path in the output directory. It differs from
	// Name only when a collision forced conflict naming.
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
}

// Provenance is the durable record shared by journals and committed records.
type Provenance struct {
	Version      int               `json:"version"`
	TaskID       string            `json:"task_id"`
	AttemptID    string            `json:"attempt_id"`
	Recipe       string            `json:"recipe,omitempty"`
	Inputs       []InputRecord     `json:"inputs"`
	Params       map[string]string `json:"params,omitempty"`
	ParamsDigest string            `json:"params_digest"`
	Outputs      []OutputRecord    `json:"outputs"`
}

// Result is returned by Run. It is also the value used when an already
// committed task is observed again.
type Result struct {
	TaskID     string
	AttemptID  string
	Outputs    []OutputRecord
	Provenance Provenance
}

// State is the durable transaction state.
type State string

const (
	StateStaging    State = "staging"
	StatePrepared   State = "prepared"
	StateCommitting State = "committing"
	StateCommitted  State = "committed"
	StateAborted    State = "aborted"
)

// journal is the write-ahead transaction log. It is persisted atomically and
// fsynced before any staged file can become visible in the output tree.
type journal struct {
	Provenance
	State     State     `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	StageDir  string    `json:"stage_dir"`
	Error     string    `json:"error,omitempty"`
}

// record is the immutable committed provenance document.
type record struct {
	Provenance
	CommittedAt time.Time `json:"committed_at"`
	JournalID   string    `json:"journal_id"`
}

// RecoveryAction describes what recovery did for one transaction.
type RecoveryAction string

const (
	RecoveredCommitted  RecoveryAction = "committed"
	RecoveredRolledBack RecoveryAction = "rolled_back"
	RecoveredCleaned    RecoveryAction = "cleaned"
	RecoveredSkipped    RecoveryAction = "skipped_active"
)

// RecoveryEntry reports the outcome for one journal.
type RecoveryEntry struct {
	Journal string
	TaskID  string
	State   State
	Action  RecoveryAction
	Err     error
}

// RecoveryReport is returned after scanning all transaction journals.
type RecoveryReport struct {
	Entries []RecoveryEntry
}

func (r RecoveryReport) Err() error {
	var errs []error
	for _, entry := range r.Entries {
		if entry.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Journal, entry.Err))
		}
	}
	return errors.Join(errs...)
}

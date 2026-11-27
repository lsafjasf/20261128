// Package autosave implements incremental, crash-safe autosave snapshots and
// the manifest that chains them together for recovery.
package autosave

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"editorautosave/project"
)

const (
	// KindBase is a self-contained snapshot holding full project state.
	KindBase = "base"
	// KindIncremental holds only operations since the previous snapshot.
	KindIncremental = "incremental"

	manifestName = "manifest.json"
	archiveDir   = "archive"
	tmpSuffix    = ".tmp"
)

// Pointer locates one snapshot file relative to the recovery directory.
type Pointer struct {
	ID   string `json:"id"`
	Seq  int    `json:"seq"`
	Kind string `json:"kind"`
	File string `json:"file"`
}

// Snapshot is one link in the recovery chain.
type Snapshot struct {
	ID         string              `json:"id"`
	ProjectID  string              `json:"project_id"`
	Kind       string              `json:"kind"`
	Seq        int                 `json:"seq"` // last applied operation seq
	BaseSeq    int                 `json:"base_seq,omitempty"`
	ParentID   string              `json:"parent_id,omitempty"`
	PrevID     string              `json:"prev_id,omitempty"`
	CreatedAt  string              `json:"created_at"`
	StateHash  string              `json:"state_hash"`
	Operations []project.Operation `json:"operations,omitempty"`
	Project    *project.Project    `json:"project,omitempty"` // base snapshots only
	Media      []project.MediaRef  `json:"media"`
	Checksum   string              `json:"checksum"`
}

// Manifest is the durable index of the recovery chain. It is written
// atomically only after its snapshot data has been fsynced, and records the
// last successfully written snapshot.
type Manifest struct {
	ProjectID      string    `json:"project_id"`
	ChainID        string    `json:"chain_id"`
	Head           *Pointer  `json:"head,omitempty"`
	Base           *Pointer  `json:"base,omitempty"`
	Snapshots      []Pointer `json:"snapshots"`
	LastSuccessAt  string    `json:"last_success_at"`
	LastSuccessSeq int       `json:"last_success_seq"`
	UpdatedAt      string    `json:"updated_at"`
}

// Store manages snapshots inside a recovery directory.
type Store struct {
	dir       string
	baseEvery int
	now       func() time.Time
	idGen     func() string
}

// Option customizes a Store.
type Option func(*Store)

// WithBaseEvery sets how many incremental snapshots are appended before the
// chain is compacted into a new base. Zero disables automatic compaction.
func WithBaseEvery(n int) Option {
	return func(s *Store) { s.baseEvery = n }
}

// WithClock injects a clock (used by tests).
func WithClock(now func() time.Time) Option {
	return func(s *Store) { s.now = now }
}

// WithIDGen injects an ID generator (used by tests).
func WithIDGen(gen func() string) Option {
	return func(s *Store) { s.idGen = gen }
}

// Open creates the recovery directory if needed and reconciles any snapshot
// files left behind by a crash before the manifest could be updated.
func Open(dir string, opts ...Option) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create recovery dir: %w", err)
	}
	s := &Store{dir: dir, baseEvery: 10, now: time.Now, idGen: randomID}
	for _, opt := range opts {
		opt(s)
	}
	if _, err := os.Stat(s.path(manifestName)); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err := s.writeManifest(&Manifest{ChainID: s.idGen(), UpdatedAt: s.now().UTC().Format(time.RFC3339Nano)}); err != nil {
			return nil, err
		}
	}
	if err := s.reconcile(); err != nil {
		return nil, err
	}
	if err := s.cleanStaleTempFiles(); err != nil {
		return nil, err
	}
	return s, nil
}

// Dir returns the recovery directory.
func (s *Store) Dir() string { return s.dir }

// Manifest returns a copy of the current manifest.
func (s *Store) Manifest() (Manifest, error) {
	return s.readManifest()
}

// SaveBase writes a self-contained base snapshot and makes it the new chain
// root. Previous snapshots are archived but retained for manual selection.
func (s *Store) SaveBase(p *project.Project) (*Snapshot, error) {
	snap := &Snapshot{
		ID:        s.idGen(),
		ProjectID: p.ID,
		Kind:      KindBase,
		Seq:       p.LastSeq(),
		CreatedAt: s.now().UTC().Format(time.RFC3339Nano),
		StateHash: p.StateHash(),
		Project:   p.Clone(),
		Media:     mediaList(p),
	}
	if err := s.writeSnapshot(snap); err != nil {
		return nil, err
	}

	manifest, err := s.readManifest()
	if err != nil {
		return nil, err
	}
	if err := s.archiveChain(&manifest); err != nil {
		return nil, err
	}
	ptr := Pointer{ID: snap.ID, Seq: snap.Seq, Kind: KindBase, File: s.snapRel(snap)}
	manifest.ProjectID = p.ID
	manifest.Base = &ptr
	manifest.Head = &ptr
	manifest.Snapshots = []Pointer{ptr}
	s.markSuccess(&manifest, snap)
	if err := s.writeManifest(&manifest); err != nil {
		return nil, err
	}
	return snap, nil
}

// AppendIncremental writes an incremental snapshot containing only the
// operations newer than the chain head. A base must already exist and the
// operations must continue the recorded sequence without gaps.
func (s *Store) AppendIncremental(p *project.Project) (*Snapshot, error) {
	manifest, err := s.readManifest()
	if err != nil {
		return nil, err
	}
	if manifest.Base == nil || manifest.Head == nil {
		return nil, fmt.Errorf("cannot append incremental snapshot: no base snapshot")
	}
	newOps := operationsAfter(p, manifest.Head.Seq)
	if len(newOps) == 0 {
		return nil, fmt.Errorf("no new operations since head seq %d", manifest.Head.Seq)
	}
	if newOps[0].Seq != manifest.Head.Seq+1 {
		return nil, fmt.Errorf("%w: head seq %d, next op seq %d", project.ErrSeqGap, manifest.Head.Seq, newOps[0].Seq)
	}

	snap := &Snapshot{
		ID:         s.idGen(),
		ProjectID:  p.ID,
		Kind:       KindIncremental,
		Seq:        p.LastSeq(),
		BaseSeq:    manifest.Base.Seq,
		ParentID:   manifest.Base.ID,
		PrevID:     manifest.Head.ID,
		CreatedAt:  s.now().UTC().Format(time.RFC3339Nano),
		StateHash:  p.StateHash(),
		Operations: newOps,
		Media:      mediaList(p),
	}
	if err := s.writeSnapshot(snap); err != nil {
		return nil, err
	}

	ptr := Pointer{ID: snap.ID, Seq: snap.Seq, Kind: KindIncremental, File: s.snapRel(snap)}
	manifest.Head = &ptr
	manifest.Snapshots = append(manifest.Snapshots, ptr)
	s.markSuccess(&manifest, snap)
	if err := s.writeManifest(&manifest); err != nil {
		return nil, err
	}

	incrementals := 0
	for _, sp := range manifest.Snapshots {
		if sp.Kind == KindIncremental {
			incrementals++
		}
	}
	if s.baseEvery > 0 && incrementals >= s.baseEvery {
		if _, err := s.SaveBase(p); err != nil {
			return snap, fmt.Errorf("compaction failed after snapshot %s: %w", snap.ID, err)
		}
	}
	return snap, nil
}

// Auto appends an incremental snapshot when a base exists, otherwise seeds
// the chain with a base snapshot.
func (s *Store) Auto(p *project.Project) (*Snapshot, error) {
	manifest, err := s.readManifest()
	if err != nil {
		return nil, err
	}
	if manifest.Base == nil {
		return s.SaveBase(p)
	}
	return s.AppendIncremental(p)
}

func (s *Store) markSuccess(manifest *Manifest, snap *Snapshot) {
	now := s.now().UTC().Format(time.RFC3339Nano)
	manifest.LastSuccessAt = now
	manifest.LastSuccessSeq = snap.Seq
	manifest.UpdatedAt = now
}

func operationsAfter(p *project.Project, seq int) []project.Operation {
	out := make([]project.Operation, 0)
	for _, op := range p.Operations {
		if op.Seq > seq {
			out = append(out, op)
		}
	}
	return out
}

func mediaList(p *project.Project) []project.MediaRef {
	out := make([]project.MediaRef, 0, len(p.Media))
	for _, ref := range p.Media {
		out = append(out, *ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Store) snapRel(snap *Snapshot) string {
	return fmt.Sprintf("snap-%06d-%s.json", snap.Seq, snap.ID[:8])
}

func (s *Store) path(rel string) string { return filepath.Join(s.dir, rel) }

func checksum(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

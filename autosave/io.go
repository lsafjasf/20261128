package autosave

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"editorautosave/project"
)

// ErrChecksum is returned when a snapshot file fails its integrity check.
var ErrChecksum = errors.New("snapshot checksum mismatch")

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("random id: %v", err))
	}
	return hex.EncodeToString(b[:])
}

// writeSnapshot serializes the snapshot, stamps its checksum, then writes the
// file via a temp file + fsync + atomic rename so a crash can never leave a
// partially written snapshot visible to recovery.
func (s *Store) writeSnapshot(snap *Snapshot) error {
	unsigned, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	// Checksum is computed over the canonical bytes with an empty checksum
	// field, then the on-disk bytes carry the same layout with the value
	// filled in, so verification can hash the exact bytes on disk.
	sum := checksum(unsigned)
	snap.Checksum = sum
	raw := replaceChecksum(unsigned, "", sum)
	return atomicWrite(s.path(s.snapRel(snap)), raw)
}

// replaceChecksum swaps the checksum value in canonical snapshot JSON.
// Passing an empty oldValue targets the empty "checksum": "" field.
func replaceChecksum(raw []byte, oldValue, newValue string) []byte {
	from := []byte(`"checksum": "` + oldValue + `"`)
	to := []byte(`"checksum": "` + newValue + `"`)
	idx := bytesIndex(raw, from)
	if idx < 0 {
		return raw
	}
	out := make([]byte, 0, len(raw)+len(newValue))
	out = append(out, raw[:idx]...)
	out = append(out, to...)
	out = append(out, raw[idx+len(from):]...)
	return out
}

func bytesIndex(raw, sep []byte) int {
	for i := 0; i <= len(raw)-len(sep); i++ {
		if string(raw[i:i+len(sep)]) == string(sep) {
			return i
		}
	}
	return -1
}

func atomicWrite(path string, data []byte) error {
	tmp := path + tmpSuffix
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return syncDir(filepath.Dir(path))
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if closeErr := d.Close(); err == nil {
		err = closeErr
	}
	return err
}

func (s *Store) writeManifest(m *Manifest) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return atomicWrite(s.path(manifestName), raw)
}

func (s *Store) readManifest() (Manifest, error) {
	var m Manifest
	raw, err := os.ReadFile(s.path(manifestName))
	if err != nil {
		return m, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("parse manifest: %w", err)
	}
	return m, nil
}

// ReadSnapshot loads and verifies one snapshot by relative path.
func (s *Store) ReadSnapshot(rel string) (*Snapshot, error) {
	raw, err := os.ReadFile(s.path(rel))
	if err != nil {
		return nil, fmt.Errorf("read snapshot %s: %w", rel, err)
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("parse snapshot %s: %w", rel, err)
	}
	stamped := snap.Checksum
	// Reconstruct the exact unsigned byte layout and checksum it, so any
	// tampering with the on-disk bytes (including trailing whitespace) fails.
	snap.Checksum = ""
	canonical, err := json.MarshalIndent(&snap, "", "  ")
	if err != nil {
		return nil, err
	}
	unsigned := replaceChecksum(raw, stamped, "")
	if checksum(unsigned) != stamped || string(canonical) != string(unsigned) {
		return nil, fmt.Errorf("%w: %s", ErrChecksum, rel)
	}
	return &snap, nil
}

// Replay rebuilds project state from a base snapshot and its following
// incrementals, verifying sequence contiguity and state hashes at every link.
func (s *Store) Replay(chain []Pointer) (*project.Project, error) {
	if len(chain) == 0 {
		return nil, errors.New("empty recovery chain")
	}
	var state *project.Project
	for i, ptr := range chain {
		snap, err := s.ReadSnapshot(ptr.File)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			if snap.Kind != KindBase || snap.Project == nil {
				return nil, fmt.Errorf("chain root %s is not a base snapshot", ptr.ID)
			}
			state = snap.Project.Clone()
		} else {
			if snap.Kind != KindIncremental {
				return nil, fmt.Errorf("unexpected %s snapshot at chain position %d", snap.Kind, i)
			}
			for _, op := range snap.Operations {
				if err := state.Apply(op); err != nil {
					return nil, fmt.Errorf("replay op seq %d: %w", op.Seq, err)
				}
			}
		}
		if state.LastSeq() != snap.Seq {
			return nil, fmt.Errorf("snapshot %s claims seq %d but state is at %d", snap.ID, snap.Seq, state.LastSeq())
		}
		if state.StateHash() != snap.StateHash {
			return nil, fmt.Errorf("%w: state hash after snapshot %s", ErrChecksum, snap.ID)
		}
	}
	return state, nil
}

// reconcile handles a crash between "snapshot file written" and "manifest
// updated": a fully valid, contiguous orphan snapshot is adopted; anything
// corrupt or non-contiguous is moved into archive/ instead of being trusted.
func (s *Store) reconcile() error {
	manifest, err := s.readManifest()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	var orphans []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "snap-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		orphans = append(orphans, name)
	}
	if len(orphans) == 0 {
		return nil
	}

	registered := make(map[string]int, len(manifest.Snapshots))
	for i, ptr := range manifest.Snapshots {
		registered[ptr.File] = i
	}
	changed := false
	for _, name := range orphans {
		snap, err := s.ReadSnapshot(name)
		idx, isRegistered := registered[name]
		if err != nil {
			// A corrupt file already listed in the manifest means the chain
			// head (or a later link) is unusable. Roll back to the good
			// prefix, then quarantine the corrupt file.
			if isRegistered {
				manifest = rollbackManifest(manifest, idx)
			}
			if quarantineErr := s.quarantine(name); quarantineErr != nil {
				return quarantineErr
			}
			changed = true
			continue
		}
		if isRegistered {
			continue // known, checksum-valid snapshot
		}
		if !s.continues(snap, &manifest) {
			if quarantineErr := s.quarantine(name); quarantineErr != nil {
				return quarantineErr
			}
			changed = true
			continue
		}
		ptr := Pointer{ID: snap.ID, Seq: snap.Seq, Kind: snap.Kind, File: name}
		manifest.Snapshots = append(manifest.Snapshots, ptr)
		if manifest.Head == nil || snap.Seq > manifest.Head.Seq {
			manifest.Head = &ptr
		}
		manifest.LastSuccessAt = snap.CreatedAt
		manifest.LastSuccessSeq = snap.Seq
		changed = true
	}
	if changed {
		return s.writeManifest(&manifest)
	}
	return nil
}

// rollbackManifest truncates the chain to keep only links strictly before the
// corrupted index and re-points head/base/last-success at the good prefix.
func rollbackManifest(manifest Manifest, badIndex int) Manifest {
	good := manifest.Snapshots[:badIndex]
	manifest.Snapshots = good
	if len(good) == 0 {
		manifest.Head = nil
		manifest.Base = nil
		manifest.LastSuccessSeq = 0
		manifest.LastSuccessAt = ""
		return manifest
	}
	last := good[len(good)-1]
	head := last
	manifest.Head = &head
	manifest.LastSuccessSeq = last.Seq
	if last.Kind == KindBase {
		base := last
		manifest.Base = &base
	}
	return manifest
}

func (s *Store) continues(snap *Snapshot, manifest *Manifest) bool {
	if manifest.Head == nil {
		return snap.Kind == KindBase
	}
	if snap.Kind != KindIncremental || snap.PrevID != manifest.Head.ID {
		return false
	}
	return snap.Seq > manifest.Head.Seq
}

func (s *Store) quarantine(name string) error {
	if err := os.MkdirAll(s.path(archiveDir), 0o755); err != nil {
		return err
	}
	dst := filepath.Join(archiveDir, filepath.Base(name))
	if err := os.Rename(s.path(name), s.path(dst)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncDir(s.dir)
}

// cleanStaleTempFiles removes .tmp files left behind when a crash happened
// mid-write; they were never renamed into place and cannot be valid snapshots.
func (s *Store) cleanStaleTempFiles() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), tmpSuffix) {
			continue
		}
		if err := os.Remove(s.path(entry.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove stale temp file %s: %w", entry.Name(), err)
		}
	}
	return syncDir(s.dir)
}

// archiveChain moves every snapshot of the superseded chain into archive/.
func (s *Store) archiveChain(manifest *Manifest) error {
	if err := os.MkdirAll(s.path(archiveDir), 0o755); err != nil {
		return err
	}
	for _, ptr := range manifest.Snapshots {
		if err := s.quarantine(ptr.File); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

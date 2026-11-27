package rendercache

import (
	"fmt"
	"sync"
)

// Registry indexes the exact bytes currently associated with each resource
// version. Put may replace the bytes for an existing version; the returned
// fingerprint changes, so every dependent cache key changes as well.
type Registry struct {
	mu      sync.RWMutex
	entries map[ResourceRef]registryEntry
}

type registryEntry struct {
	fingerprint Fingerprint
	data        []byte
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[ResourceRef]registryEntry)}
}

// Put adds or replaces a resource version and returns its content fingerprint.
// The registry owns a copy of data.
func (r *Registry) Put(res Resource) (ResolvedResource, error) {
	if err := res.Ref.validate(); err != nil {
		return ResolvedResource{}, err
	}
	if len(res.Data) == 0 {
		return ResolvedResource{}, fmt.Errorf("%w: resource data is empty", ErrInvalidResourceRef)
	}

	entry := registryEntry{
		fingerprint: FingerprintBytes(res.Data),
		data:        append([]byte(nil), res.Data...),
	}

	r.mu.Lock()
	if r.entries == nil {
		r.entries = make(map[ResourceRef]registryEntry)
	}
	r.entries[res.Ref] = entry
	r.mu.Unlock()

	return ResolvedResource{
		Ref:         res.Ref,
		Fingerprint: entry.fingerprint,
		Size:        int64(len(entry.data)),
	}, nil
}

// Remove makes a version unknown. It does not delete cached entries; their
// keys become unreachable, which prevents stale output from being served.
func (r *Registry) Remove(ref ResourceRef) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[ref]; !ok {
		return false
	}
	delete(r.entries, ref)
	return true
}

// Resolve returns the current immutable snapshot for one version.
func (r *Registry) Resolve(ref ResourceRef) (ResolvedResource, error) {
	if err := ref.validate(); err != nil {
		if ref.Version == "" {
			return ResolvedResource{}, &UnknownResourceError{Ref: ref}
		}
		return ResolvedResource{}, err
	}

	r.mu.RLock()
	entry, ok := r.entries[ref]
	r.mu.RUnlock()
	if !ok {
		return ResolvedResource{}, &UnknownResourceError{Ref: ref}
	}
	return ResolvedResource{
		Ref:         ref,
		Fingerprint: entry.fingerprint,
		Size:        int64(len(entry.data)),
	}, nil
}

// ResolveAll resolves a set of direct stage inputs under one read lock. This
// keeps a cache key from mixing fingerprints observed before and after a
// concurrent registry update.
func (r *Registry) ResolveAll(refs []ResourceRef) (map[ResourceRef]ResolvedResource, error) {
	snapshots, err := r.SnapshotAll(refs)
	if err != nil {
		return nil, err
	}
	resolved := make(map[ResourceRef]ResolvedResource, len(snapshots))
	for ref, snapshot := range snapshots {
		resolved[ref] = snapshot.ResolvedResource
	}
	return resolved, nil
}

// SnapshotAll resolves and copies all requested inputs under one read lock.
func (r *Registry) SnapshotAll(refs []ResourceRef) (map[ResourceRef]ResourceSnapshot, error) {
	unique := make(map[ResourceRef]struct{}, len(refs))
	ordered := make([]ResourceRef, 0, len(refs))
	for _, ref := range refs {
		if _, ok := unique[ref]; ok {
			continue
		}
		unique[ref] = struct{}{}
		ordered = append(ordered, ref)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshots := make(map[ResourceRef]ResourceSnapshot, len(ordered))
	for _, ref := range ordered {
		if err := ref.validate(); err != nil {
			if ref.Version == "" {
				return nil, &UnknownResourceError{Ref: ref}
			}
			return nil, err
		}
		entry, ok := r.entries[ref]
		if !ok {
			return nil, &UnknownResourceError{Ref: ref}
		}
		snapshots[ref] = ResourceSnapshot{
			ResolvedResource: ResolvedResource{
				Ref:         ref,
				Fingerprint: entry.fingerprint,
				Size:        int64(len(entry.data)),
			},
			Data: append([]byte(nil), entry.data...),
		}
	}
	return snapshots, nil
}

// Load returns a copy of the bytes for a resolved version. Stage runners use
// this to consume exactly the content that was fingerprinted.
func (r *Registry) Load(ref ResourceRef) ([]byte, error) {
	if err := ref.validate(); err != nil {
		if ref.Version == "" {
			return nil, &UnknownResourceError{Ref: ref}
		}
		return nil, err
	}

	r.mu.RLock()
	entry, ok := r.entries[ref]
	if ok {
		entry.data = append([]byte(nil), entry.data...)
	}
	r.mu.RUnlock()
	if !ok {
		return nil, &UnknownResourceError{Ref: ref}
	}
	return entry.data, nil
}

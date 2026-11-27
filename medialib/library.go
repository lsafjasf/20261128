package medialib

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Errors returned for broken preconditions; callers can use errors.Is.
var (
	ErrNotFound         = errors.New("medialib: object not found")
	ErrDuplicateID      = errors.New("medialib: duplicate stable id")
	ErrBadRange         = errors.New("medialib: invalid source range")
	ErrRangeOutOfBounds = errors.New("medialib: source range outside master bounds")
	ErrDanglingRef      = errors.New("medialib: dangling reference")
	ErrInUse            = errors.New("medialib: object still referenced")
	ErrWrongKind        = errors.New("medialib: id kind mismatch")
)

// Library is the identity registry. All lookups are by stable ID; names and
// source ranges are treated purely as mutable attributes.
type Library struct {
	mu        sync.RWMutex
	masters   map[ID]*MasterMedia
	subclips  map[ID]*Subclip
	instances map[ID]*Instance
}

func NewLibrary() *Library {
	return &Library{
		masters:   map[ID]*MasterMedia{},
		subclips:  map[ID]*Subclip{},
		instances: map[ID]*Instance{},
	}
}

// AddMaster registers a source asset and returns its stable ID.
func (l *Library) AddMaster(name, filePath string, duration int64) (ID, error) {
	if duration <= 0 {
		return ID{}, fmt.Errorf("%w: duration must be positive", ErrBadRange)
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	id := newID(KindMaster)
	l.masters[id] = &MasterMedia{ID: id, Name: name, FilePath: filePath, Duration: duration}
	return id, nil
}

// CutSubclip creates a NEW identity even when an identical (master, range)
// pair already exists. Same source range never means "same subclip".
func (l *Library) CutSubclip(master ID, name string, r TimeRange, markers map[string]string) (ID, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	m, ok := l.masters[master]
	if !ok {
		return ID{}, fmt.Errorf("%w: master %s", ErrDanglingRef, master)
	}
	if !r.Valid() {
		return ID{}, fmt.Errorf("%w: %+v", ErrBadRange, r)
	}
	if r.End > m.Duration {
		return ID{}, fmt.Errorf("%w: range end %d > duration %d", ErrRangeOutOfBounds, r.End, m.Duration)
	}

	id := newID(KindSubclip)
	cp := copyMarkers(markers)
	l.subclips[id] = &Subclip{ID: id, Master: master, Name: name, Range: r, Markers: cp}
	return id, nil
}

// AddInstance places a subclip on a timeline and returns the instance ID.
func (l *Library) AddInstance(subclip ID, timeline string, at, length int64) (ID, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.subclips[subclip]; !ok {
		return ID{}, fmt.Errorf("%w: subclip %s", ErrDanglingRef, subclip)
	}
	if length <= 0 {
		return ID{}, fmt.Errorf("%w: instance length must be positive", ErrBadRange)
	}

	id := newID(KindInstance)
	l.instances[id] = &Instance{ID: id, Subclip: subclip, Timeline: timeline, At: at, Length: length}
	return id, nil
}

// Rename changes mutable display data only; IDs and all references stay put.
func (l *Library) Rename(id ID, name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	switch id.Kind {
	case KindMaster:
		m, ok := l.masters[id]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		m.Name = name
	case KindSubclip:
		s, ok := l.subclips[id]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		s.Name = name
	default:
		return fmt.Errorf("%w: %s is not renameable here", ErrWrongKind, id)
	}
	return nil
}

// SetMarker mutates marker metadata without touching identity or references.
func (l *Library) SetMarker(subclip ID, key, value string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	s, ok := l.subclips[subclip]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, subclip)
	}
	s.Markers[key] = value
	return nil
}

// Retrim changes the source range of an existing subclip. The subclip ID is
// preserved, so every instance keeps pointing at it. The new range must stay
// within the owning master's bounds.
func (l *Library) Retrim(subclip ID, r TimeRange) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	s, ok := l.subclips[subclip]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, subclip)
	}
	if !r.Valid() {
		return fmt.Errorf("%w: %+v", ErrBadRange, r)
	}
	m, ok := l.masters[s.Master]
	if !ok {
		return fmt.Errorf("%w: master %s", ErrDanglingRef, s.Master)
	}
	if r.End > m.Duration {
		return fmt.Errorf("%w: range end %d > duration %d", ErrRangeOutOfBounds, r.End, m.Duration)
	}
	s.Range = r
	return nil
}

// DuplicateSubclip deep-copies a subclip into a NEW identity. The copy gets
// its own name/markers and never aliases the original's mutable state.
func (l *Library) DuplicateSubclip(src ID, newName string) (ID, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	s, ok := l.subclips[src]
	if !ok {
		return ID{}, fmt.Errorf("%w: %s", ErrNotFound, src)
	}
	id := newID(KindSubclip)
	l.subclips[id] = &Subclip{
		ID:      id,
		Master:  s.Master,
		Name:    newName,
		Range:   s.Range,
		Markers: copyMarkers(s.Markers),
	}
	return id, nil
}

// DeleteMaster removes a master. It fails while subclips still reference it
// unless cascade is true, in which case dependent subclips and their timeline
// instances are removed first.
func (l *Library) DeleteMaster(id ID, cascade bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.masters[id]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	dependents := l.subclipsLocked(func(s *Subclip) bool { return s.Master == id })
	if len(dependents) > 0 && !cascade {
		return fmt.Errorf("%w: master %s has %d subclip(s)", ErrInUse, id, len(dependents))
	}
	for _, sid := range dependents {
		l.deleteSubclipLocked(sid, true)
	}
	delete(l.masters, id)
	return nil
}

// DeleteSubclip removes a subclip. Non-cascade deletion fails while timeline
// instances still reference it.
func (l *Library) DeleteSubclip(id ID, cascade bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.subclips[id]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return l.deleteSubclipLocked(id, cascade)
}

func (l *Library) deleteSubclipLocked(id ID, cascade bool) error {
	users := l.instancesLocked(func(ins *Instance) bool { return ins.Subclip == id })
	if len(users) > 0 && !cascade {
		return fmt.Errorf("%w: subclip %s has %d instance(s)", ErrInUse, id, len(users))
	}
	for _, iid := range users {
		delete(l.instances, iid)
	}
	delete(l.subclips, id)
	return nil
}

// DeleteInstance removes one timeline placement; the subclip itself is untouched.
func (l *Library) DeleteInstance(id ID) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.instances[id]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	delete(l.instances, id)
	return nil
}

// VerifyIntegrity asserts every reference resolves and every range is in
// bounds. It is the post-operation invariant check used after mutate tests.
func (l *Library) VerifyIntegrity() error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, s := range l.subclips {
		m, ok := l.masters[s.Master]
		if !ok {
			return fmt.Errorf("%w: subclip %s -> master %s", ErrDanglingRef, s.ID, s.Master)
		}
		if !s.Range.Valid() || s.Range.End > m.Duration {
			return fmt.Errorf("%w: subclip %s range %+v vs duration %d",
				ErrRangeOutOfBounds, s.ID, s.Range, m.Duration)
		}
	}
	for _, ins := range l.instances {
		if _, ok := l.subclips[ins.Subclip]; !ok {
			return fmt.Errorf("%w: instance %s -> subclip %s", ErrDanglingRef, ins.ID, ins.Subclip)
		}
	}
	return nil
}

func (r TimeRange) String() string {
	return fmt.Sprintf("[%d,%d)", r.Start, r.End)
}

// --- read accessors (return defensive copies) ---

func (l *Library) Master(id ID) (MasterMedia, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	m, ok := l.masters[id]
	if !ok {
		return MasterMedia{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *m, nil
}

func (l *Library) Subclip(id ID) (Subclip, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	s, ok := l.subclips[id]
	if !ok {
		return Subclip{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	cp := *s
	cp.Markers = copyMarkers(s.Markers)
	return cp, nil
}

func (l *Library) Instance(id ID) (Instance, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	ins, ok := l.instances[id]
	if !ok {
		return Instance{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *ins, nil
}

// InstancesOf returns the instance IDs currently referencing a subclip.
func (l *Library) InstancesOf(subclip ID) []ID {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.instancesLocked(func(ins *Instance) bool { return ins.Subclip == subclip })
}

// SubclipsOf returns the subclip IDs cut from a master, including duplicates
// that share an identical source range.
func (l *Library) SubclipsOf(master ID) []ID {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.subclipsLocked(func(s *Subclip) bool { return s.Master == master })
}

func (l *Library) subclipsLocked(match func(*Subclip) bool) []ID {
	var out []ID
	for id, s := range l.subclips {
		if match(s) {
			out = append(out, id)
		}
	}
	sortIDs(out)
	return out
}

func (l *Library) instancesLocked(match func(*Instance) bool) []ID {
	var out []ID
	for id, ins := range l.instances {
		if match(ins) {
			out = append(out, id)
		}
	}
	sortIDs(out)
	return out
}

func sortIDs(ids []ID) {
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
}

func copyMarkers(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

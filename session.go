package captionsession

import (
	"sync"
)

// InitialVersion is the version before any op has been produced.
const InitialVersion int64 = 0

// defaultSubBuffer is the per-subscriber channel capacity. A viewer that
// cannot keep up is marked stalled instead of applying backpressure to the
// whole session.
const defaultSubBuffer = 64

// Session is a versioned, append-only caption timeline for one live stream.
// Every state transition gets a strictly increasing version number and is
// fanned out to all subscribers, so late joiners and reconnecting clients
// converge to exactly what the live audience saw.
type Session struct {
	mu        sync.RWMutex
	clock     Clock
	version   int64
	watermark int64
	segments  map[string]Segment
	order     []string
	log       []Op
	base      int64 // version of the last compaction snapshot (InitialVersion if none)
	subs      map[*subscriber]struct{}
}

// NewSession creates an empty session using the wall clock.
func NewSession() *Session {
	return newSession(systemClock{})
}

func newSession(clock Clock) *Session {
	return &Session{
		clock:    clock,
		segments: map[string]Segment{},
		subs:     map[*subscriber]struct{}{},
	}
}

// Version returns the latest committed op version.
func (s *Session) Version() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

// Watermark returns the current committed presentation PTS (milliseconds).
func (s *Session) Watermark() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.watermark
}

// Upsert inserts a provisional ASR segment or revises an existing one.
// Revisions targeting already-displayed content are rejected so future edits
// can never flow backwards into presented captions.
func (s *Session) Upsert(seg Segment) (Op, error) {
	if !validSegment(seg) {
		return Op{}, ErrInvalidSegment
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.segments[seg.ID]; exists && existing.EndPTS <= s.watermark {
		return Op{}, ErrSegmentFrozen
	}
	if _, exists := s.segments[seg.ID]; !exists && seg.EndPTS <= s.watermark {
		// A brand-new segment that belongs entirely in the past cannot be
		// shown consistently to the current audience.
		return Op{}, ErrSegmentFrozen
	}
	op := Op{Kind: OpUpsert, WallTime: s.nowMS(), Segment: &seg}
	if _, exists := s.segments[seg.ID]; !exists {
		s.order = append(s.order, seg.ID)
	}
	s.segments[seg.ID] = seg
	return s.appendLocked(op), nil
}

// Replace applies an operator's local text correction to a single segment.
// Only the target segment changes: neighbors keep their identity and
// position, so the transcript never reflows. It fails once the segment has
// been displayed.
func (s *Session) Replace(segmentID, newText string) (Op, error) {
	if segmentID == "" {
		return Op{}, ErrInvalidSegment
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seg, ok := s.segments[segmentID]
	if !ok {
		return Op{}, ErrSegmentNotFound
	}
	if seg.EndPTS <= s.watermark {
		return Op{}, ErrSegmentFrozen
	}
	old := seg.Text
	seg.Text = newText
	s.segments[segmentID] = seg
	op := Op{
		Kind:      OpReplace,
		WallTime:  s.nowMS(),
		SegmentID: segmentID,
		OldText:   old,
		NewText:   newText,
	}
	return s.appendLocked(op), nil
}

// AdvanceWatermark commits presentation up to pts. All segments ending at or
// before pts become immutable. Advancing backwards is a no-op.
func (s *Session) AdvanceWatermark(pts int64) (Op, bool, error) {
	if pts < 0 {
		return Op{}, false, ErrInvalidSegment
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if pts <= s.watermark {
		return Op{}, false, nil
	}
	s.watermark = pts
	op := Op{Kind: OpWatermark, WallTime: s.nowMS(), Watermark: pts}
	return s.appendLocked(op), true, nil
}

func (s *Session) appendLocked(op Op) Op {
	s.version++
	op.Version = s.version
	s.log = append(s.log, op)
	s.fanoutLocked(op)
	return op
}

func (s *Session) nowMS() int64 {
	return s.clock.Now().UnixMilli()
}

// State materializes the session at its current version.
func (s *Session) State() *State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := NewState()
	for _, id := range s.order {
		seg := s.segments[id]
		st.segments[id] = seg
		st.order = append(st.order, id)
	}
	// Replay the materialized view through the same deterministic path used
	// by clients, so a freshly initialized state carries the right version.
	snap := Snapshot{BaseVersion: s.version, Watermark: s.watermark, Segments: st.Segments()}
	out, err := Apply(&snap, nil)
	if err != nil {
		panic(err) // unreachable: snapshot built internally
	}
	out.Version = s.version
	return out
}

// Snapshot materializes the session for resync and compaction.
func (s *Session) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *Session) snapshotLocked() Snapshot {
	segs := make([]Segment, 0, len(s.order))
	for _, id := range s.order {
		segs = append(segs, s.segments[id])
	}
	return Snapshot{
		BaseVersion: s.version,
		Watermark:   s.watermark,
		Segments:    segs,
		TakenAt:     s.nowMS(),
	}
}

// Compact rewrites the op log prefix into a snapshot, keeping at most the
// last retain ops. Reconnecting clients that ask for versions older than
// the new base must resync from the snapshot instead of replaying.
func (s *Session) Compact(retain int) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if retain < 0 {
		retain = 0
	}
	snap := s.snapshotLocked()
	if retain >= len(s.log) {
		return snap, nil
	}
	cut := len(s.log) - retain
	s.log = append([]Op(nil), s.log[cut:]...)
	s.base = snap.BaseVersion
	return snap, nil
}

// Replay returns the op suffix strictly after fromVersion, along with the
// retained snapshot base. If fromVersion is older than the retained base the
// caller cannot replay incrementally and must resync from the snapshot.
func (s *Session) Replay(fromVersion int64) (Snapshot, []Op, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ops := make([]Op, 0, len(s.log))
	for _, op := range s.log {
		if op.Version > fromVersion {
			ops = append(ops, op)
		}
	}
	var snap Snapshot
	if s.base > InitialVersion {
		// Compacted log: expose the retained snapshot explicitly so callers
		// can materialize it before applying the incremental suffix.
		snap = s.snapshotLocked()
	}
	if fromVersion < s.base {
		return snap, nil, ErrResync
	}
	return snap, ops, nil
}

// Subscribe attaches a new live viewer. It immediately receives a snapshot
// event at the current version, followed by every subsequent op, so a late
// joiner reconstructs exactly what earlier viewers already saw.
func (s *Session) Subscribe() *Subscriber {
	inner := newSubscriber()
	s.mu.Lock()
	snap := s.snapshotLocked()
	s.subs[inner] = struct{}{}
	s.mu.Unlock()
	inner.deliver(Event{Op: Op{Kind: OpSnapshot, Version: snap.BaseVersion, Snapshot: &snap}})
	return &Subscriber{inner: inner, session: s}
}

// SubscribeFrom resumes a disconnected viewer from the last version it
// processed. If the requested version was compacted away, the first event is
// a resync snapshot; otherwise only missing ops are replayed.
func (s *Session) SubscribeFrom(lastVersion int64) *Subscriber {
	inner := newSubscriber()
	s.mu.Lock()
	aheadOfLog := lastVersion > s.version
	if lastVersion < s.base || aheadOfLog {
		snap := s.snapshotLocked()
		s.subs[inner] = struct{}{}
		s.mu.Unlock()
		inner.deliver(Event{Op: Op{Kind: OpSnapshot, Version: snap.BaseVersion, Snapshot: &snap}})
		if aheadOfLog {
			inner.deliver(Event{Err: ErrResync})
		}
		return &Subscriber{inner: inner, session: s}
	}
	backlog := make([]Op, 0)
	for _, op := range s.log {
		if op.Kind != OpSnapshot && op.Version > lastVersion {
			backlog = append(backlog, op)
		}
	}
	s.subs[inner] = struct{}{}
	s.mu.Unlock()
	for _, op := range backlog {
		inner.deliver(Event{Op: op})
	}
	return &Subscriber{inner: inner, session: s}
}

func (s *Session) fanoutLocked(op Op) {
	for sub := range s.subs {
		ok := sub.tryDeliver(Event{Op: op})
		if !ok {
			sub.markStalled()
		}
	}
}

func (s *Session) removeSubscriber(sub *subscriber) {
	s.mu.Lock()
	delete(s.subs, sub)
	s.mu.Unlock()
}

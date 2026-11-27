package captionsession

import (
	"errors"
	"strconv"
	"testing"
)

// materializeSubscriber drains currently buffered events into a State.
func materializeSubscriber(t *testing.T, sub *Subscriber) *State {
	t.Helper()
	var st *State
drain:
	for {
		select {
		case ev := <-sub.Events():
			if ev.Err != nil {
				t.Fatalf("unexpected subscriber error: %v", ev.Err)
			}
			if ev.Op.Kind == OpSnapshot {
				var err error
				st, err = Apply(ev.Op.Snapshot, nil)
				if err != nil {
					t.Fatal(err)
				}
				continue
			}
			if st == nil {
				st = NewState()
			}
			if err := st.apply(ev.Op); err != nil {
				t.Fatal(err)
			}
		default:
			break drain
		}
	}
	return st
}

func TestLateJoinerSeesFullHistory(t *testing.T) {
	s := newTestSession()
	_, _ = s.Upsert(Segment{ID: "a", StartPTS: 0, EndPTS: 1000, Text: "one"})
	_, _ = s.Upsert(Segment{ID: "b", StartPTS: 1000, EndPTS: 2000, Text: "two"})

	sub := s.Subscribe()
	// Live traffic after subscription must also arrive.
	_, _ = s.Upsert(Segment{ID: "c", StartPTS: 2000, EndPTS: 3000, Text: "three"})

	st := materializeSubscriber(t, sub)
	sub.Close()
	live := s.State()
	if !statesEqual(st, live) {
		t.Fatalf("late joiner state %+v != live %+v", st.Segments(), live.Segments())
	}
}

func TestReconnectIncrementalResume(t *testing.T) {
	s := newTestSession()
	opA, _ := s.Upsert(Segment{ID: "a", StartPTS: 0, EndPTS: 1000, Text: "one"})
	_, _ = s.Upsert(Segment{ID: "b", StartPTS: 1000, EndPTS: 2000, Text: "two"})

	// Client disconnects after processing opA.
	lastVersion := opA.Version
	_, _ = s.Replace("b", "TWO")
	_, _, _ = s.AdvanceWatermark(2000)

	sub := s.SubscribeFrom(lastVersion)
	opsReceived := 0
	var st *State
drain:
	for {
		select {
		case ev := <-sub.Events():
			if ev.Err != nil {
				t.Fatalf("unexpected error: %v", ev.Err)
			}
			if ev.Op.Kind == OpSnapshot {
				t.Fatalf("incremental resume must not resend snapshot")
			}
			if st == nil {
				st = NewState()
			}
			_ = st.apply(ev.Op)
			opsReceived++
		default:
			break drain
		}
	}
	// Missing ops: opB (upsert happened after disconnect) + replace + watermark.
	if opsReceived != 3 {
		t.Fatalf("replayed %d ops, want 2", opsReceived)
	}

	// Full reconstruction from v0 must match the server regardless.
	_, allOps, err := s.Replay(InitialVersion)
	if err != nil {
		t.Fatal(err)
	}
	full, err := Apply(nil, allOps)
	if err != nil {
		t.Fatal(err)
	}
	if !statesEqual(full, s.State()) {
		t.Fatalf("full replay diverged: %+v", full.Segments())
	}
}

func TestReconnectAfterCompactionResyncsFromSnapshot(t *testing.T) {
	s := newTestSession()
	old, _ := s.Upsert(Segment{ID: "a", StartPTS: 0, EndPTS: 1000, Text: "one"})
	_, _ = s.Upsert(Segment{ID: "b", StartPTS: 1000, EndPTS: 2000, Text: "two"})

	snap, err := s.Compact(1)
	if err != nil {
		t.Fatal(err)
	}
	if snap.BaseVersion != s.Version() {
		t.Fatalf("snapshot base %d != version %d", snap.BaseVersion, s.Version())
	}

	// Replay from before the compaction base must demand a resync.
	if _, _, err := s.Replay(old.Version - 1); !errors.Is(err, ErrResync) {
		t.Fatalf("want ErrResync, got %v", err)
	}

	// SubscribeFrom returns a snapshot instead of stale incremental ops.
	sub := s.SubscribeFrom(old.Version - 1)
	ev := <-sub.Events()
	if ev.Op.Kind != OpSnapshot {
		t.Fatalf("want snapshot event after compaction, got %s", ev.Op.Kind)
	}
	st, err := Apply(ev.Op.Snapshot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !statesEqual(st, s.State()) {
		t.Fatalf("snapshot state diverged: %+v", st.Segments())
	}
	sub.Close()

	// New traffic after compaction still versions correctly.
	op, _ := s.Upsert(Segment{ID: "c", StartPTS: 2000, EndPTS: 3000, Text: "three"})
	if op.Version != s.Version() {
		t.Fatalf("post-compaction versioning broken")
	}
}

func TestStalledSubscriberRequiresResync(t *testing.T) {
	s := newTestSession()
	sub := s.Subscribe()
	// Never drain the channel; overflow the 64-deep buffer.
	for i := 0; i < defaultSubBuffer*2; i++ {
		_, _ = s.Upsert(Segment{
			ID:       segID(i),
			StartPTS: int64(i) * 1000,
			EndPTS:   int64(i+1) * 1000,
			Text:     "x",
		})
	}
	if !sub.Stalled() {
		t.Fatalf("slow consumer should be marked stalled")
	}
	var sawResync bool
drain:
	for {
		select {
		case ev := <-sub.Events():
			if errors.Is(ev.Err, ErrResync) {
				sawResync = true
				break drain
			}
		default:
			break drain
		}
	}
	if !sawResync {
		t.Fatalf("stalled subscriber must receive ErrResync event")
	}
	// Resync path: fresh snapshot converges immediately.
	sub2 := s.SubscribeFrom(s.Version())
	sub2.Close()
}

func segID(i int) string {
	return "seg-" + strconv.Itoa(i)
}

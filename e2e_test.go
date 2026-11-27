package captionsession

import (
	"errors"
	"testing"
	"time"
)

// TestEndToEndCaptionSession exercises the full scenario from the brief:
//   - captions lag speech by ~2s and are compensated via the playhead
//   - an operator corrects one line locally without reflowing others
//   - a viewer disconnects and reconnects, converging to the live state
//   - revisions of already-displayed content are rejected (no backfill)
func TestEndToEndCaptionSession(t *testing.T) {
	clock := &mutableClock{t: time.UnixMilli(60_000)}
	session := newSession(clock)
	// Freeze the estimator at the observed 2s lag for deterministic timing.
	est := NewLatencyEstimator()
	est.ems = 2000
	est.samples = 100
	est.Freeze()
	scheduler := NewScheduler(session,
		WithClock(clock),
		WithAssumedDelay(2000),
		WithLatencyEstimator(est),
	)

	// A live viewer joins before any speech is captioned.
	live := session.Subscribe()
	defer live.Close()

	// ASR emits two utterances with the real ~2s pipeline lag: speech at
	// PTS 58000-59000 arrives at wall time 60000-61000.
	mustIngest(t, scheduler, Segment{ID: "u1", StartPTS: 58200, EndPTS: 59000, Text: "welcome every one"})
	mustIngest(t, scheduler, Segment{ID: "u2", StartPTS: 59000, EndPTS: 60000, Text: "to the sho"})

	// First pump at wall 60000 -> playhead 58000: nothing due yet.
	res, err := scheduler.Pump()
	if err != nil || len(res.Released) != 0 {
		t.Fatalf("premature release: %+v err=%v", res.Released, err)
	}

	// Wall 61200 -> playhead 59200: u1 displayed, u2 displayed.
	clock.advance(1200 * time.Millisecond)
	if _, err := scheduler.Pump(); err != nil {
		t.Fatal(err)
	}

	// Operator fixes u2 locally. u1 must stay byte-identical and positions
	// must not move.
	fixOp, err := scheduler.Replace("u2", "to the show")
	if err != nil {
		t.Fatalf("operator replace: %v", err)
	}
	if fixOp.Kind != OpReplace {
		t.Fatalf("want scoped replace op, got %s", fixOp.Kind)
	}
	segs := session.State().Segments()
	if segs[0].ID != "u1" || segs[1].ID != "u2" || segs[1].Text != "to the show" {
		t.Fatalf("local replace reflowed the transcript: %+v", segs)
	}

	// Viewer records its last processed version and "disconnects".
	lastVersion := fixOp.Version

	// More speech and a watermark advance freeze u1.
	clock.advance(1000 * time.Millisecond)
	mustIngest(t, scheduler, Segment{ID: "u3", StartPTS: 60000, EndPTS: 61000, Text: "tuning in"})
	if _, err := scheduler.Pump(); err != nil {
		t.Fatal(err)
	}

	// A delayed ASR correction for u1 arrives after it was displayed.
	_, err = session.Upsert(Segment{ID: "u1", StartPTS: 58200, EndPTS: 59000, Text: "welcome everyone"})
	if !errors.Is(err, ErrSegmentFrozen) {
		t.Fatalf("backfill must be rejected, got %v", err)
	}
	// And an operator edit is rejected just the same.
	if _, err := session.Replace("u1", "hijacked"); !errors.Is(err, ErrSegmentFrozen) {
		t.Fatalf("operator backfill must be rejected, got %v", err)
	}
	original, _ := session.State().Get("u1")
	if original.Text != "welcome every one" {
		t.Fatalf("displayed caption was mutated: %q", original.Text)
	}

	// Reconnect: resume from the last version the viewer processed.
	reconnected := session.SubscribeFrom(lastVersion)
	// The client still holds its pre-disconnect state; replay the retained
	// op prefix through the deterministic engine, then apply the incremental
	// suffix delivered on reconnect.
	_, prefix, err := session.Replay(InitialVersion)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := Apply(nil, prefix)
	if err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case ev := <-reconnected.Events():
			if ev.Err != nil {
				t.Fatalf("reconnect error: %v", ev.Err)
			}
			if ev.Op.Kind == OpSnapshot {
				replayed, err = Apply(ev.Op.Snapshot, nil)
				if err != nil {
					t.Fatal(err)
				}
				continue
			}
			if err := replayed.apply(ev.Op); err != nil {
				t.Fatal(err)
			}
		default:
			reconnected.Close()
			goto done
		}
	}
done:

	// Historical viewer state and current server state must be identical at
	// the same version.
	if !statesEqual(replayed, session.State()) {
		t.Fatalf("reconnected viewer diverged:\nreplay=%+v\nlive=%+v",
			replayed.Segments(), session.State().Segments())
	}

	// The live subscriber's independently materialized view must also match.
	liveView := drainToState(t, live)
	if !statesEqual(liveView, session.State()) {
		t.Fatalf("live viewer diverged from server: %+v", liveView.Segments())
	}
}

func mustIngest(t *testing.T, sc *Scheduler, seg Segment) {
	t.Helper()
	if err := sc.Ingest(seg); err != nil {
		t.Fatalf("ingest %s: %v", seg.ID, err)
	}
}

func drainToState(t *testing.T, sub *Subscriber) *State {
	t.Helper()
	var st *State
	for {
		select {
		case ev := <-sub.Events():
			if ev.Err != nil {
				t.Fatalf("subscriber error: %v", ev.Err)
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
			if st == nil {
				st = NewState()
			}
			return st
		}
	}
}

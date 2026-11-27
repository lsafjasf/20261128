package captionsession

import (
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
)

// mutableClock is a test clock that only moves forward explicitly.
type mutableClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *mutableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *mutableClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestScheduler() (*Scheduler, *mutableClock) {
	// Start far enough from the epoch that playhead never clamps to zero.
	clock := &mutableClock{t: time.UnixMilli(60_000)}
	session := newSession(clock)
	// Fixed estimator: observations are accepted but the test pins the
	// compensated delay at 2000ms for predictable playheads.
	est := NewLatencyEstimator()
	est.ems = 2000
	est.samples = 100
	est.Freeze()
	sc := NewScheduler(session, WithClock(clock), WithAssumedDelay(2000), WithLatencyEstimator(est))
	return sc, clock
}

func TestSchedulerReleasesInPTSOrder(t *testing.T) {
	sc, clock := newTestScheduler()
	// Initial wall = 60000, assumed delay 2000 -> initial playhead = 58000.
	// Both segments start strictly after the initial playhead.
	err := sc.Ingest(Segment{ID: "b", StartPTS: 59000, EndPTS: 60000, Text: "two"})
	if err != nil {
		t.Fatal(err)
	}
	if err := sc.Ingest(Segment{ID: "a", StartPTS: 58200, EndPTS: 59000, Text: "one"}); err != nil {
		t.Fatal(err)
	}

	// playhead 58000: neither segment is due yet.
	res, err := sc.Pump()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Released) != 0 {
		t.Fatalf("nothing should be released yet, got %+v", res.Released)
	}

	// Wall 60500 -> playhead 58500: only a (start 58200) is due.
	clock.advance(500 * time.Millisecond)
	res, _ = sc.Pump()
	if len(res.Released) != 1 || res.Released[0].Segment.ID != "a" {
		t.Fatalf("want segment a released first, got %+v", res.Released)
	}

	// Wall 61000 -> playhead 59000: b becomes due and releases second.
	clock.advance(1000 * time.Millisecond)
	res, _ = sc.Pump()
	if len(res.Released) != 1 || res.Released[0].Segment.ID != "b" {
		t.Fatalf("want segment b released second, got %+v", res.Released)
	}
	if got := sc.session.State().Segments(); got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("timeline order wrong: %+v", got)
	}
}

func TestSchedulerFreezesDisplayedSegments(t *testing.T) {
	sc, clock := newTestScheduler()
	// playhead starts at 58000; place the segment just ahead of it.
	if err := sc.Ingest(Segment{ID: "a", StartPTS: 58050, EndPTS: 59000, Text: "v1"}); err != nil {
		t.Fatal(err)
	}
	// Wall 61500 -> playhead 59500: segment released, then frozen.
	clock.advance(1500 * time.Millisecond)
	if _, err := sc.Pump(); err != nil {
		t.Fatal(err)
	}
	if sc.session.Watermark() != 59500 {
		t.Fatalf("watermark = %d, want 59500", sc.session.Watermark())
	}
	if _, err := sc.Replace("a", "v2"); !errors.Is(err, ErrSegmentFrozen) {
		t.Fatalf("post-display replace must freeze, got %v", err)
	}

	// A late event whose window already passed must be dropped silently at
	// release time rather than injected retroactively.
	if err := sc.Ingest(Segment{ID: "late", StartPTS: 50000, EndPTS: 51000, Text: "x"}); err != nil {
		t.Fatal(err)
	}
	res, err := sc.Pump()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Released) != 0 {
		t.Fatalf("late segment must not be released, got %+v", res.Released)
	}
	if _, ok := sc.session.State().Get("late"); ok {
		t.Fatalf("late segment leaked into state")
	}
}

func TestSchedulerLearnsDelay(t *testing.T) {
	clock := &mutableClock{t: time.UnixMilli(60_000)}
	session := newSession(clock)
	sc := NewScheduler(session, WithClock(clock), WithAssumedDelay(2000))
	// Observed lag is 1500ms instead of the assumed 2000ms.
	for i := 0; i < 25; i++ {
		clock.advance(time.Second)
		endPTS := clock.Now().UnixMilli() - 1500
		_ = sc.Ingest(Segment{
			ID:       "seg-" + strconv.Itoa(i),
			StartPTS: endPTS - 900,
			EndPTS:   endPTS,
			Text:     "x",
		})
		_, _ = sc.Pump()
	}
	got := sc.EstimatedDelayMS()
	if got < 1400 || got > 1600 {
		t.Fatalf("learned delay %dms, want ~1500ms", got)
	}
}

func TestSchedulerRunStopLoop(t *testing.T) {
	clock := &mutableClock{t: time.UnixMilli(60_000)}
	session := newSession(clock)
	est := NewLatencyEstimator()
	est.ems = 2000
	est.samples = 100
	est.Freeze()
	scheduler := NewScheduler(session,
		WithClock(clock),
		WithTick(10*time.Millisecond),
		WithLatencyEstimator(est),
	)
	if err := scheduler.Ingest(Segment{ID: "x", StartPTS: 58050, EndPTS: 59000, Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	go scheduler.Run()
	clock.advance(1500 * time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := session.State().Get("x"); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	scheduler.Stop()
	if _, ok := session.State().Get("x"); !ok {
		t.Fatalf("background loop never released the segment")
	}
}

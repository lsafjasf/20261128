package captionsession

import (
	"container/heap"
	"time"
)

// Scheduler sits in front of a Session. ASR events may arrive late or out of
// order; it buffers them, estimates pipeline delay, releases each event once
// the playhead reaches its PTS, and advances the session watermark so
// displayed segments freeze.
type Scheduler struct {
	session   *Session
	clock     Clock
	estimator *LatencyEstimator
	assumed   int64
	queue     eventHeap
	tick      time.Duration
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// SchedulerOption customizes a scheduler.
type SchedulerOption func(*Scheduler)

// WithClock injects a clock, mainly for deterministic tests.
func WithClock(c Clock) SchedulerOption {
	return func(sc *Scheduler) { sc.clock = c }
}

// WithAssumedDelay sets the delay used before the estimator has samples
// (default 2000ms, matching the observed caption lag).
func WithAssumedDelay(d time.Duration) SchedulerOption {
	return func(sc *Scheduler) { sc.assumed = d.Milliseconds() }
}

// WithLatencyEstimator injects an estimator, mainly for deterministic tests.
func WithLatencyEstimator(e *LatencyEstimator) SchedulerOption {
	return func(sc *Scheduler) { sc.estimator = e }
}

// WithTick sets the background pump interval (default 100ms).
func WithTick(d time.Duration) SchedulerOption {
	return func(sc *Scheduler) { sc.tick = d }
}

// NewScheduler builds a scheduler for the session.
func NewScheduler(session *Session, opts ...SchedulerOption) *Scheduler {
	sc := &Scheduler{
		session:   session,
		clock:     systemClock{},
		estimator: NewLatencyEstimator(),
		assumed:   2000,
		tick:      100 * time.Millisecond,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	for _, opt := range opts {
		opt(sc)
	}
	return sc
}

// Ingest buffers an incoming ASR segment. The event is timestamped at
// arrival and released in PTS order by the pump, so late/out-of-order
// deliveries still produce an ordered timeline.
func (sc *Scheduler) Ingest(seg Segment) error {
	if !validSegment(seg) {
		return ErrInvalidSegment
	}
	sc.estimator.Observe(sc.clock.Now().UnixMilli(), seg.EndPTS)
	heap.Push(&sc.queue, scheduledEvent{seg: seg, arrivedAt: sc.clock.Now().UnixMilli()})
	return nil
}

// Replace delegates an operator's local correction to the session, enforcing
// the same no-backfill freeze rule.
func (sc *Scheduler) Replace(segmentID, newText string) (Op, error) {
	return sc.session.Replace(segmentID, newText)
}

// EstimatedDelayMS exposes the current delay compensation value.
func (sc *Scheduler) EstimatedDelayMS() int64 {
	return sc.estimator.EstimateMS(sc.assumed)
}

// PumpResult summarizes one pump cycle, useful in tests and metrics.
type PumpResult struct {
	Playhead       int64
	Released       []Op
	WatermarkOp    Op
	WatermarkMoved bool
}

// Pump runs one scheduling cycle at the current wall time. It is idempotent
// and safe to call concurrently with session reads.
func (sc *Scheduler) Pump() (PumpResult, error) {
	now := sc.clock.Now().UnixMilli()
	delay := sc.estimator.EstimateMS(sc.assumed)
	play := Playhead(now, delay)
	res := PumpResult{Playhead: play}

	for sc.queue.Len() > 0 && sc.queue[0].seg.StartPTS <= play {
		ev := heap.Pop(&sc.queue).(scheduledEvent)
		op, err := sc.session.Upsert(ev.seg)
		if err == ErrSegmentFrozen {
			// The segment crossed the watermark while queued: the audience
			// already passed this point, so it must not appear retroactively.
			continue
		}
		if err != nil {
			return res, err
		}
		res.Released = append(res.Released, op)
	}

	op, moved, err := sc.session.AdvanceWatermark(play)
	if err != nil {
		return res, err
	}
	res.WatermarkOp, res.WatermarkMoved = op, moved
	return res, nil
}

// Run starts the background pump loop.
func (sc *Scheduler) Run() {
	ticker := time.NewTicker(sc.tick)
	defer ticker.Stop()
	for {
		select {
		case <-sc.stopCh:
			close(sc.doneCh)
			return
		case <-ticker.C:
			_, _ = sc.Pump()
		}
	}
}

// Stop terminates the background loop and waits for it to finish.
func (sc *Scheduler) Stop() {
	select {
	case <-sc.stopCh:
	default:
		close(sc.stopCh)
	}
	<-sc.doneCh
}

type scheduledEvent struct {
	seg       Segment
	arrivedAt int64
}

type eventHeap []scheduledEvent

func (h eventHeap) Len() int { return len(h) }

func (h eventHeap) Less(i, j int) bool {
	if h[i].seg.StartPTS != h[j].seg.StartPTS {
		return h[i].seg.StartPTS < h[j].seg.StartPTS
	}
	return h[i].arrivedAt < h[j].arrivedAt
}

func (h eventHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *eventHeap) Push(x any) { *h = append(*h, x.(scheduledEvent)) }

func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

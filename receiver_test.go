package st2110

import (
	"sync/atomic"
	"testing"
	"time"
)

var testFormat = Format{Encoding: EncodingL16, Channels: 2, SampleRate: 48000}

// frameBytes is one 1 ms packet at 48 kHz stereo L16: 48 frames * 4 bytes.
const frameBytes = 48 * 4

type harness struct {
	clock  *PTPClock
	now    time.Time
	recv   *Receiver
	events []Event
}

func newHarness(policy DegradePolicy) *harness {
	h := &harness{now: time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)}
	h.clock = NewPTPClock(func() time.Time { return h.now })
	h.clock.SetLocked(true)
	r, err := NewReceiver(testFormat, h.clock, policy)
	if err != nil {
		panic(err)
	}
	r.OnEvent = func(e Event) { h.events = append(h.events, e) }
	h.recv = r
	return h
}

func (h *harness) hasEvent(k EventKind) bool {
	for _, e := range h.events {
		if e.Kind == k {
			return true
		}
	}
	return false
}

func TestReceiverHappyPath(t *testing.T) {
	h := newHarness(DegradeMute)
	for i := uint16(0); i < 10; i++ {
		out := h.recv.Push(LegA, buildRTP(i, uint32(i)*48, frameBytes))
		if out == nil || out.Degraded {
			t.Fatalf("packet %d dropped or degraded", i)
		}
		if out.Samples != 48 {
			t.Fatalf("samples=%d want 48", out.Samples)
		}
		h.now = h.now.Add(time.Millisecond)
	}
	s := h.recv.Stats()
	if s.Accepted != 10 || s.TimestampDiscontinuities != 0 || s.DriftEvents != 0 {
		t.Fatalf("stats=%+v", s)
	}
}

func TestReceiverDuplicateAndLate(t *testing.T) {
	h := newHarness(DegradeMute)
	h.recv.Push(LegA, buildRTP(0, 0, frameBytes))
	h.now = h.now.Add(time.Millisecond)
	if out := h.recv.Push(LegB, buildRTP(0, 0, frameBytes)); out != nil {
		t.Fatal("duplicate from leg B must be dropped")
	}
	// Jump far ahead so seq 1 falls beyond the reorder window.
	h.recv.Push(LegA, buildRTP(200, 200*48, frameBytes))
	if out := h.recv.Push(LegA, buildRTP(1, 48, frameBytes)); out != nil {
		t.Fatal("late packet must be dropped")
	}
	s := h.recv.Stats()
	if s.Duplicates != 1 || s.Late != 1 {
		t.Fatalf("stats=%+v", s)
	}
	if !h.hasEvent(EventPacketLate) {
		t.Fatal("missing EventPacketLate")
	}
}

func TestReceiverTimestampDiscontinuity(t *testing.T) {
	h := newHarness(DegradeMute)
	h.recv.Push(LegA, buildRTP(0, 1000, frameBytes))
	h.now = h.now.Add(time.Millisecond)
	// Timestamp jumps by 96 instead of the 48 samples carried previously.
	h.recv.Push(LegA, buildRTP(1, 1096, frameBytes))
	if !h.hasEvent(EventTimestampDiscontinuity) {
		t.Fatal("missing discontinuity event")
	}
	if h.recv.Stats().TimestampDiscontinuities != 1 {
		t.Fatalf("stats=%+v", h.recv.Stats())
	}
}

func TestReceiverDriftDetection(t *testing.T) {
	h := newHarness(DegradeMute)
	h.recv.Push(LegA, buildRTP(0, 0, frameBytes))
	// Sender advances RTP by 1 ms but PTP time advanced 10 ms: not locked
	// to the same grandmaster.
	h.now = h.now.Add(10 * time.Millisecond)
	h.recv.Push(LegA, buildRTP(1, 48, frameBytes))
	if !h.hasEvent(EventTimestampDrift) {
		t.Fatal("missing drift event")
	}
}

// TestDegradedMuteNeverTouchesWallClock proves that while PTP is unlocked
// the receiver neither emits media nor consults any time source.
func TestDegradedMuteNeverTouchesWallClock(t *testing.T) {
	var clockCalls atomic.Int64
	h := &harness{now: time.Now()}
	h.clock = NewPTPClock(func() time.Time {
		clockCalls.Add(1)
		return h.now
	})
	h.clock.SetLocked(true)
	r, _ := NewReceiver(testFormat, h.clock, DegradeMute)
	var events []Event
	r.OnEvent = func(e Event) { events = append(events, e) }

	r.Push(LegA, buildRTP(0, 0, frameBytes))
	h.clock.SetLocked(false)
	for i := uint16(1); i <= 5; i++ {
		if out := r.Push(LegA, buildRTP(i, uint32(i)*48, frameBytes)); out != nil {
			t.Fatal("muted receiver must not emit while unlocked")
		}
	}
	s := r.Stats()
	if s.MutedPackets != 5 || s.PTPUnlocks != 1 {
		t.Fatalf("stats=%+v", s)
	}
	// No Now() calls may happen while unlocked (only the locked packet may
	// have touched the source).
	if n := clockCalls.Load(); n > 1 {
		t.Fatalf("clock source consulted %d times; wall clock must never substitute PTP", n)
	}
	if !hasKind(events, EventPTPLockLost) {
		t.Fatal("missing lock-lost event")
	}

	// Recovery re-anchors and resumes.
	h.clock.SetLocked(true)
	if out := r.Push(LegA, buildRTP(6, 6*48, frameBytes)); out == nil {
		t.Fatal("receiver must resume after relock")
	}
	if !hasKind(events, EventPTPLockRecovered) {
		t.Fatal("missing recovery event")
	}
}

func hasKind(events []Event, k EventKind) bool {
	for _, e := range events {
		if e.Kind == k {
			return true
		}
	}
	return false
}

func TestDegradedPassThroughIsFlagged(t *testing.T) {
	h := newHarness(DegradePassThrough)
	h.recv.Push(LegA, buildRTP(0, 0, frameBytes))
	h.clock.SetLocked(false)
	out := h.recv.Push(LegA, buildRTP(1, 48, frameBytes))
	if out == nil || !out.Degraded {
		t.Fatalf("pass-through output must be flagged degraded: %+v", out)
	}
	// De-duplication still works while degraded.
	if out := h.recv.Push(LegB, buildRTP(1, 48, frameBytes)); out != nil {
		t.Fatal("duplicate must be dropped even while degraded")
	}
	s := h.recv.Stats()
	if s.DegradedPackets != 1 || s.Duplicates != 1 {
		t.Fatalf("stats=%+v", s)
	}
}

func TestReceiverRejectsNilClock(t *testing.T) {
	if _, err := NewReceiver(testFormat, nil, DegradeMute); err == nil {
		t.Fatal("nil clock must be rejected: no silent wall-clock fallback")
	}
}

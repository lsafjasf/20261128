package st2110

import (
	"fmt"
	"time"
)

// DegradePolicy selects what happens to media while PTP is unlocked.
type DegradePolicy int

const (
	// DegradeMute drops all media while PTP is unlocked.
	DegradeMute DegradePolicy = iota
	// DegradePassThrough forwards media immediately, flagged Degraded,
	// with all timing verification disabled. It never substitutes a
	// local clock for playout scheduling.
	DegradePassThrough
)

// EventKind identifies a Receiver event.
type EventKind int

const (
	EventPTPLockLost EventKind = iota
	EventPTPLockRecovered
	EventTimestampDiscontinuity // RTP timestamp step != previous packet's sample count
	EventTimestampDrift         // RTP/PTP alignment error beyond tolerance
	EventPayloadError
	EventPacketLate
)

type Event struct {
	Kind   EventKind
	Leg    Leg
	Seq    uint16
	Detail string
}

// Stats aggregates receiver counters.
type Stats struct {
	Accepted                 uint64
	Duplicates               uint64
	Reordered                uint64
	Late                     uint64
	PayloadErrors            uint64
	TimestampDiscontinuities uint64
	DriftEvents              uint64
	DegradedPackets          uint64 // forwarded while PTP unlocked
	MutedPackets             uint64 // dropped while PTP unlocked
	PTPUnlocks               uint64
}

// Output is one accepted, de-duplicated audio packet.
type Output struct {
	Packet   Packet
	Samples  int
	Degraded bool // true: PTP unlocked, timing not verifiable
}

// Receiver ties parsing, redundancy merging, timestamp validation and
// PTP lock supervision together. It is not safe for concurrent use;
// serialize Push calls per stream.
type Receiver struct {
	Format         Format
	Clock          Clock
	Policy         DegradePolicy
	ReorderWindow  int
	DriftTolerance time.Duration
	OnEvent        func(Event)

	merger   *Merger
	stats    Stats
	degraded bool

	tsInit      bool
	lastTS      uint32
	lastSamples uint32

	refInit bool
	refTS   uint32
	refTime time.Time
}

func NewReceiver(f Format, c Clock, policy DegradePolicy) (*Receiver, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("st2110: a Clock is required; wall clock fallback is not supported")
	}
	r := &Receiver{
		Format:         f,
		Clock:          c,
		Policy:         policy,
		ReorderWindow:  64,
		DriftTolerance: 2 * time.Millisecond,
	}
	r.merger = NewMerger(r.ReorderWindow)
	return r, nil
}

func (r *Receiver) Stats() Stats { return r.stats }

// Lost returns sequence numbers missing on both redundant legs.
func (r *Receiver) Lost() uint64 { return r.merger.Lost() }

func (r *Receiver) LegStats(leg Leg) LegStats { return r.merger.LegStats(leg) }

func (r *Receiver) emit(e Event) {
	if r.OnEvent != nil {
		r.OnEvent(e)
	}
}

// Push ingests one raw RTP datagram from a redundant leg. It returns the
// accepted output, or nil when the packet was dropped (duplicate, late,
// malformed, or muted while PTP is unlocked).
func (r *Receiver) Push(leg Leg, datagram []byte) *Output {
	pkt, err := ParsePacket(datagram)
	if err != nil {
		r.stats.PayloadErrors++
		r.emit(Event{Kind: EventPayloadError, Leg: leg, Detail: err.Error()})
		return nil
	}
	samples, err := r.Format.SamplesInPayload(len(pkt.Payload))
	if err != nil {
		r.stats.PayloadErrors++
		r.emit(Event{Kind: EventPayloadError, Leg: leg, Seq: pkt.SequenceNumber, Detail: err.Error()})
		return nil
	}

	if !r.Clock.Locked() {
		return r.pushDegraded(leg, pkt, samples)
	}
	if r.degraded {
		// Lock recovered: re-anchor the RTP<->PTP mapping from scratch.
		r.degraded = false
		r.refInit = false
		r.emit(Event{Kind: EventPTPLockRecovered, Leg: leg, Seq: pkt.SequenceNumber})
	}

	accepted, class := r.merger.Push(leg, pkt)
	switch class {
	case ClassDuplicate:
		r.stats.Duplicates++
	case ClassReordered:
		r.stats.Reordered++
	case ClassLate:
		r.stats.Late++
		r.emit(Event{Kind: EventPacketLate, Leg: leg, Seq: pkt.SequenceNumber,
			Detail: "sequence number beyond reorder window"})
	}
	if !accepted {
		return nil
	}
	r.stats.Accepted++

	r.checkTimestampContinuity(pkt, uint32(samples))
	r.checkDrift(pkt)

	return &Output{Packet: pkt, Samples: samples}
}

// pushDegraded handles media while PTP is unlocked. Timing checks are
// impossible, so packets are either muted or passed through flagged;
// sequence-based de-duplication still runs.
func (r *Receiver) pushDegraded(leg Leg, pkt Packet, samples int) *Output {
	if !r.degraded {
		r.degraded = true
		r.stats.PTPUnlocks++
		r.emit(Event{Kind: EventPTPLockLost, Leg: leg, Seq: pkt.SequenceNumber,
			Detail: "PTP lock lost; entering degraded mode, no wall-clock fallback"})
	}
	if r.Policy == DegradeMute {
		r.stats.MutedPackets++
		return nil
	}
	accepted, _ := r.merger.Push(leg, pkt)
	if !accepted {
		r.stats.Duplicates++
		return nil
	}
	r.stats.DegradedPackets++
	return &Output{Packet: pkt, Samples: samples, Degraded: true}
}

// checkTimestampContinuity enforces the ST 2110-30 rule that the RTP
// timestamp advances by exactly the previous packet's sample count.
func (r *Receiver) checkTimestampContinuity(pkt Packet, samples uint32) {
	if r.tsInit && pkt.Timestamp != r.lastTS+r.lastSamples {
		r.stats.TimestampDiscontinuities++
		r.emit(Event{
			Kind: EventTimestampDiscontinuity,
			Seq:  pkt.SequenceNumber,
			Detail: fmt.Sprintf("timestamp %d, expected %d (last %d + %d samples)",
				pkt.Timestamp, r.lastTS+r.lastSamples, r.lastTS, r.lastSamples),
		})
	}
	r.tsInit = true
	r.lastTS = pkt.Timestamp
	r.lastSamples = samples
}

// checkDrift compares the packet's RTP timestamp against the PTP-locked
// arrival time. Both clocks share the PTP epoch in a locked domain, so a
// persistent offset beyond tolerance means the sender is not locked to
// the same grandmaster.
func (r *Receiver) checkDrift(pkt Packet) {
	now, ok := r.Clock.Now()
	if !ok {
		return // lock raced away; degraded path handles the next packet
	}
	if !r.refInit {
		r.refInit = true
		r.refTS = pkt.Timestamp
		r.refTime = now
		return
	}
	expected := r.refTime.Add(time.Duration(r.Format.SamplesToDuration(pkt.Timestamp - r.refTS)))
	skew := now.Sub(expected)
	if skew < 0 {
		skew = -skew
	}
	if skew > r.DriftTolerance {
		r.stats.DriftEvents++
		r.emit(Event{
			Kind:   EventTimestampDrift,
			Seq:    pkt.SequenceNumber,
			Detail: fmt.Sprintf("RTP/PTP skew %s exceeds tolerance %s", skew, r.DriftTolerance),
		})
	}
}

package st2110

// Leg identifies one path of an ST 2022-7 redundant pair.
type Leg int

const (
	LegA Leg = iota
	LegB
)

func (l Leg) String() string {
	if l == LegB {
		return "B"
	}
	return "A"
}

// LegStats counts packets observed on one redundant path.
type LegStats struct {
	Received  uint64
	Accepted  uint64 // first copy of a sequence number, from this leg
	Duplicate uint64 // copy already delivered via either leg
	Late      uint64
	Reordered uint64
}

// Merger reconstructs one stream from a redundant ST 2022-7 pair:
// the first copy of each sequence number wins, later copies are dropped.
type Merger struct {
	tracker *SeqTracker
	legs    [2]LegStats
}

func NewMerger(reorderWindow int) *Merger {
	return &Merger{tracker: NewSeqTracker(reorderWindow)}
}

// Push classifies pkt from the given leg. accepted=true means this is the
// first usable copy and the caller should forward it.
func (m *Merger) Push(leg Leg, pkt Packet) (accepted bool, class Class) {
	stats := &m.legs[leg]
	stats.Received++
	class = m.tracker.Classify(pkt.SequenceNumber)
	switch class {
	case ClassInOrder, ClassReordered:
		stats.Accepted++
		if class == ClassReordered {
			stats.Reordered++
		}
		return true, class
	case ClassDuplicate:
		stats.Duplicate++
	case ClassLate:
		stats.Late++
	}
	return false, class
}

func (m *Merger) LegStats(leg Leg) LegStats { return m.legs[leg] }

// Lost is the count of sequence numbers never seen on either leg.
func (m *Merger) Lost() uint64 { return m.tracker.Lost() }

package st2110

// Class is the arrival classification of one sequence number.
type Class int

const (
	ClassInOrder   Class = iota // extends the sequence (gap, if any, counts as lost)
	ClassDuplicate              // already seen
	ClassReordered              // behind max, inside the reorder window
	ClassLate                   // behind max, beyond the reorder window
)

const ringBits = 2048

// SeqTracker classifies uint16 sequence numbers with wrap-around handling.
// reorderWindow must be well below ringBits (2048).
type SeqTracker struct {
	window  int
	maxSeq  uint16
	started bool
	seen    [ringBits / 64]uint64
	lost    uint64
}

func NewSeqTracker(reorderWindow int) *SeqTracker {
	if reorderWindow < 0 || reorderWindow > ringBits/2 {
		reorderWindow = 64
	}
	return &SeqTracker{window: reorderWindow}
}

func (t *SeqTracker) mark(seq uint16) {
	t.seen[(uint32(seq)%ringBits)/64] |= 1 << (uint32(seq) % 64)
}

func (t *SeqTracker) isSeen(seq uint16) bool {
	return t.seen[(uint32(seq)%ringBits)/64]&(1<<(uint32(seq)%64)) != 0
}

// Classify records seq and returns its class. Forward jumps accumulate
// the skipped numbers into Lost().
func (t *SeqTracker) Classify(seq uint16) Class {
	if !t.started {
		t.started = true
		t.maxSeq = seq
		t.mark(seq)
		return ClassInOrder
	}
	forward := seq - t.maxSeq // uint16 wrap arithmetic
	switch {
	case forward == 0:
		return ClassDuplicate
	case forward < 0x8000:
		t.lost += uint64(forward) - 1
		t.maxSeq = seq
		t.mark(seq)
		return ClassInOrder
	}
	behind := t.maxSeq - seq
	if int(behind) > t.window {
		// Beyond the window the ring bitmap may alias; never consult it here.
		return ClassLate
	}
	if t.isSeen(seq) {
		return ClassDuplicate
	}
	t.mark(seq)
	return ClassReordered
}

// Lost is the cumulative count of sequence numbers skipped by forward jumps.
func (t *SeqTracker) Lost() uint64 { return t.lost }

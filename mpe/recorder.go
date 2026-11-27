package mpe

// Zone describes one MPE zone: a master channel plus a contiguous block of
// member channels that actually carry notes.
type Zone struct {
	Master  int // zero based channel
	Members []int
}

func (z Zone) contains(ch int) bool {
	for _, m := range z.Members {
		if m == ch {
			return true
		}
	}
	return false
}

// parked holds expression messages observed on a member channel while no
// note is active there. MPE sends bend/pressure before the note-on as part
// of the gesture, so the next note must inherit these values.
type parked struct {
	bend     *BendPoint
	pressure *PressurePoint
}

// Recorder turns a stream of raw MIDI events into reconstructed note
// lifecycles. It is single-goroutine: feed events sequentially through
// Process, then read Notes or call Flush.
type Recorder struct {
	lower Zone
	upper Zone

	active   map[int]*Note // member channel -> currently owned note
	finished []*Note
	parked   map[int]*parked

	nextID   int64
	lastTick int64

	// RPN tracking used to recognize MPE configuration messages (RPN 6).
	rpnMSB  map[int]int
	rpnLSB  map[int]int
	dataMSB map[int]int
	dataLSB map[int]int

	// Orphans counts note-off / expression messages that could not be
	// matched to any note. They are retained for diagnostics rather than
	// silently dropped.
	Orphans int
	// Stolen counts notes that were force-ended by channel reuse.
	Stolen int
}

// NewRecorder builds a recorder with the MPE factory default: a lower zone
// on master channel 1 (zero based 0) with all 15 remaining channels as
// members, and no upper zone.
func NewRecorder() *Recorder {
	r := &Recorder{
		active:  map[int]*Note{},
		parked:  map[int]*parked{},
		rpnMSB:  map[int]int{},
		rpnLSB:  map[int]int{},
		dataMSB: map[int]int{},
		dataLSB: map[int]int{},
	}
	r.lower = Zone{Master: 0, Members: makeRange(1, 15)}
	r.upper = Zone{Master: 15, Members: nil}
	return r
}

// NewZonedRecorder builds a recorder with explicit lower/upper member
// channel counts. A count of 0 disables that zone.
func NewZonedRecorder(lowerMembers, upperMembers int) *Recorder {
	r := NewRecorder()
	r.configure(0, lowerMembers)
	r.configure(15, upperMembers)
	return r
}

func makeRange(start, n int) []int {
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, start+i)
	}
	return out
}

// classify returns the zone owning channel ch, or nil if it is not a
// member channel of any configured zone.
func (r *Recorder) classify(ch int) *Zone {
	if r.lower.contains(ch) {
		return &r.lower
	}
	if r.upper.contains(ch) {
		return &r.upper
	}
	return nil
}

// zoneForMaster returns the zone whose master channel is ch, or nil.
func (r *Recorder) zoneForMaster(ch int) *Zone {
	if r.lower.Master == ch && len(r.lower.Members) > 0 {
		return &r.lower
	}
	if r.upper.Master == ch && len(r.upper.Members) > 0 {
		return &r.upper
	}
	return nil
}

// Notes returns every completed note in the order its lifecycle ended.
// Active notes are excluded until Flush closes them.
func (r *Recorder) Notes() []*Note {
	out := make([]*Note, len(r.finished))
	copy(out, r.finished)
	return out
}

// ActiveNotes returns notes still sounding, keyed by member channel.
func (r *Recorder) ActiveNotes() map[int]*Note {
	out := make(map[int]*Note, len(r.active))
	for ch, n := range r.active {
		out[ch] = n
	}
	return out
}

package mpe

// EventKind enumerates the channel-voice and control messages the
// recorder understands. All values are derived from raw MIDI bytes by the
// parser; callers may also construct them directly.
type EventKind int

const (
	KindUnknown EventKind = iota
	KindNoteOn
	KindNoteOff
	KindPitchBend
	KindChannelPressure // mono aftertouch
	KindPolyPressure    // polyphonic key pressure
	KindControlChange
	KindMPEConfig // synthesized by the parser when RPN 6 completes
	KindAllNotesOff
	KindAllSoundOff
)

// Event is a single timestamped MIDI channel message. Tick is application
// supplied (samples, microseconds, a sequencer position, ...); the recorder
// only requires it to be non-decreasing within a stream.
type Event struct {
	Tick    int64
	Channel int // zero based: 0..15 (MIDI channel 1..16)
	Kind    EventKind

	Note     int // note on/off and poly pressure
	Velocity int // note on/off
	Value    int // bend (14 bit), pressure (7 bit), CC (7 bit)
	CC       int // control number for KindControlChange

	// MemberChannels is populated for KindMPEConfig and records how many
	// member channels the zone starting at Channel owns.
	MemberChannels int
}

// NoteState describes how a note's life ended.
type NoteState int

const (
	// StateActive means the note is still sounding (no note off seen yet).
	StateActive NoteState = iota
	// StateClosed means an explicit note off was received.
	StateClosed
	// StateStolen means the member channel was reused before a note off
	// arrived (channel exhaustion / voice stealing).
	StateStolen
	// StateFlushed means recording ended while the note was still active.
	StateFlushed
)

// BendPoint is one pitch-bend sample belonging to a note. Value is the raw
// 14 bit MIDI value with 8192 as the centre (no bend).
type BendPoint struct {
	Tick  int64
	Value int
}

// PressurePoint is one pressure/aftertouch sample belonging to a note.
// Polyphonic pressure is keyed to the note number; channel pressure is
// forwarded to whichever note currently owns the member channel.
type PressurePoint struct {
	Tick  int64
	Value int
	Poly  bool
}

// Note is the reconstructed lifecycle of one key gesture. The identity is
// the pair (ID, MemberChannel), never the channel alone, because MPE
// controllers recycle member channels across many notes.
type Note struct {
	// ID is a monotonic identifier assigned by the recorder, one per
	// distinct note-on. This is the stable "note ID" the stream lacks.
	ID int64

	Channel int // member channel the note was assigned to
	Note    int // MIDI note number

	StartTick   int64
	EndTick     int64 // valid once State != StateActive
	Velocity    int
	OffVelocity int

	State NoteState

	// Bend and Pressure hold every expression sample observed while the
	// note was alive, plus any sample parked on the member channel between
	// the previous note-off and this note-on.
	Bend     []BendPoint
	Pressure []PressurePoint
}

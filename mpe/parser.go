package mpe

// Reader parses a raw MIDI byte stream into Events. It keeps the minimal
// state needed for running status and lets callers drive timing, so it
// works equally well over a file, a device port, or test fixtures.
type Reader struct {
	status byte
}

// NewReader returns an empty Reader.
func NewReader() *Reader { return &Reader{} }

// Parse converts one complete channel message (including its status byte,
// unless running status applies) into an Event. tick is stamped onto the
// result. System realtime/common and SysEx bytes are skipped by the caller
// via IsSystem; they return KindUnknown here.
func (rd *Reader) Parse(tick int64, msg []byte) Event {
	if len(msg) == 0 {
		return Event{Tick: tick, Kind: KindUnknown}
	}

	status := msg[0]
	if status&0x80 != 0 {
		rd.status = status
	} else {
		// Running status: reuse the previous status, first byte is data.
		status = rd.status
		msg = append([]byte{status}, msg...)
	}

	if IsSystem(status) {
		return Event{Tick: tick, Kind: KindUnknown}
	}

	ch := int(status & 0x0F)
	switch status & 0xF0 {
	case 0x90:
		return Event{Tick: tick, Channel: ch, Kind: KindNoteOn,
			Note: int(msg[1]), Velocity: int(msg[2])}
	case 0x80:
		return Event{Tick: tick, Channel: ch, Kind: KindNoteOff,
			Note: int(msg[1]), Velocity: int(msg[2])}
	case 0xE0:
		v := int(msg[1]) | int(msg[2])<<7 // little endian 14 bit
		return Event{Tick: tick, Channel: ch, Kind: KindPitchBend, Value: v}
	case 0xD0:
		return Event{Tick: tick, Channel: ch, Kind: KindChannelPressure,
			Value: int(msg[1])}
	case 0xA0:
		return Event{Tick: tick, Channel: ch, Kind: KindPolyPressure,
			Note: int(msg[1]), Value: int(msg[2])}
	case 0xB0:
		kind := KindControlChange
		if msg[1] == 123 {
			kind = KindAllNotesOff
		}
		if msg[1] == 120 {
			kind = KindAllSoundOff
		}
		return Event{Tick: tick, Channel: ch, Kind: kind,
			CC: int(msg[1]), Value: int(msg[2])}
	}
	return Event{Tick: tick, Channel: ch, Kind: KindUnknown}
}

// IsSystem reports whether a status byte is a system (non-channel) message.
func IsSystem(status byte) bool {
	return status&0xF0 == 0xF0
}

// Feed is a convenience that parses and immediately records each message.
func (r *Recorder) Feed(rd *Reader, tick int64, msg []byte) {
	ev := rd.Parse(tick, msg)
	if ev.Kind == KindUnknown {
		return
	}
	r.Process(ev)
}

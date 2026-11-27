package mpe

// expression routes bend/pressure/CC74 messages.
//
// On a member channel the message belongs to the single note currently
// assigned there. If no note is active yet the value is parked so the next
// note-on on that channel inherits it — MPE gestures routinely start with
// bend or pressure before the key goes down.
func (r *Recorder) expression(ev Event) {
	// Master channel: expression fans out to every active note owned by
	// that master (conventional, shared MIDI expression).
	if r.isMaster(ev.Channel) {
		for slot, note := range r.active {
			if slot >= 16 && note.Channel == ev.Channel {
				r.apply(note, ev)
			}
		}
		return
	}

	if r.classify(ev.Channel) == nil {
		// Channel outside any configured zone: keep the value but do not
		// let it crash the stream. Park it for a possible later note.
		r.park(ev)
		return
	}

	if note, ok := r.active[ev.Channel]; ok {
		r.apply(note, ev)
		return
	}
	r.park(ev)
}

func (r *Recorder) isMaster(ch int) bool {
	return (len(r.lower.Members) > 0 && r.lower.Master == ch) ||
		(len(r.upper.Members) > 0 && r.upper.Master == ch)
}

// apply attaches one expression sample to a note.
func (r *Recorder) apply(note *Note, ev Event) {
	switch ev.Kind {
	case KindPitchBend:
		// Polyphonic pressure on a different key than the active note does
		// not belong to this member-channel note; ignore silently.
		note.Bend = append(note.Bend, BendPoint{Tick: ev.Tick, Value: ev.Value})
	case KindChannelPressure:
		note.Pressure = append(note.Pressure, PressurePoint{
			Tick: ev.Tick, Value: ev.Value, Poly: false,
		})
	case KindPolyPressure:
		if ev.Note != note.Note {
			return
		}
		note.Pressure = append(note.Pressure, PressurePoint{
			Tick: ev.Tick, Value: ev.Value, Poly: true,
		})
	case KindControlChange:
		if ev.CC == 74 { // MPE timbre / "slide" axis, kept alongside bend
			note.Bend = append(note.Bend, BendPoint{Tick: ev.Tick, Value: ev.Value})
		}
	}
}

// park stores an expression sample on a channel with no active note.
func (r *Recorder) park(ev Event) {
	p := r.parked[ev.Channel]
	if p == nil {
		p = &parked{}
		r.parked[ev.Channel] = p
	}
	switch ev.Kind {
	case KindPitchBend:
		p.bend = &BendPoint{Tick: ev.Tick, Value: ev.Value}
	case KindChannelPressure:
		p.pressure = &PressurePoint{Tick: ev.Tick, Value: ev.Value}
	case KindPolyPressure:
		p.pressure = &PressurePoint{Tick: ev.Tick, Value: ev.Value, Poly: true}
	case KindControlChange:
		if ev.CC == 74 {
			p.bend = &BendPoint{Tick: ev.Tick, Value: ev.Value}
		}
	}
}

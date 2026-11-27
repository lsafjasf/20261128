package mpe

// Process feeds one event into the recorder. It never returns an error:
// malformed or unmatched messages are counted (see Orphans) rather than
// interrupting the stream, so a reordered or pathological sequence cannot
// lose the notes around it.
func (r *Recorder) Process(ev Event) {
	if ev.Tick < r.lastTick {
		// Out of order timestamps: keep monotonic bookkeeping while still
		// recording the event against the note it belongs to.
		ev.Tick = r.lastTick
	}
	r.lastTick = ev.Tick

	switch ev.Kind {
	case KindMPEConfig:
		r.configure(ev.Channel, ev.MemberChannels)
		return
	case KindControlChange:
		if r.handleRPN(ev) {
			return
		}
		if ev.CC == 123 {
			r.allNotesOff(ev.Channel, ev.Tick, StateFlushed)
			return
		}
		if ev.CC == 120 {
			r.allNotesOff(ev.Channel, ev.Tick, StateFlushed)
			return
		}
		r.expression(ev)
		return
	case KindAllNotesOff, KindAllSoundOff:
		r.allNotesOff(ev.Channel, ev.Tick, StateFlushed)
		return
	case KindNoteOn:
		if ev.Velocity == 0 { // MIDI convention: velocity 0 == note off
			r.noteOff(ev)
			return
		}
		r.noteOn(ev)
		return
	case KindNoteOff:
		r.noteOff(ev)
		return
	case KindPitchBend, KindChannelPressure, KindPolyPressure:
		r.expression(ev)
		return
	default:
		r.Orphans++
	}
}

// noteOn starts a new note on ev.Channel. The channel is the only handle
// the wire gives us, but it is recycled: if a note is still active there
// the previous note is stolen and fully preserved first.
func (r *Recorder) noteOn(ev Event) {
	if r.isMaster(ev.Channel) {
		// A master-channel note is a conventional (non-MPE) voice. It has
		// no member channel of its own, so it is stored directly and never
		// participates in stealing.
		r.startMasterNote(ev)
		return
	}

	if existing, ok := r.active[ev.Channel]; ok {
		// Channel exhaustion: the controller reused a member channel whose
		// previous note never received a note-off. Close that note as
		// stolen instead of overwriting and losing it.
		existing.State = StateStolen
		existing.EndTick = ev.Tick
		r.finished = append(r.finished, existing)
		r.Stolen++
		delete(r.active, ev.Channel)
	}

	note := &Note{
		ID:        r.nextID,
		Channel:   ev.Channel,
		Note:      ev.Note,
		StartTick: ev.Tick,
		Velocity:  ev.Velocity,
		State:     StateActive,
	}
	r.nextID++

	// Inherit any bend/pressure parked on this channel before the note-on.
	if p := r.parked[ev.Channel]; p != nil {
		if p.bend != nil {
			note.Bend = append(note.Bend, *p.bend)
		}
		if p.pressure != nil {
			note.Pressure = append(note.Pressure, *p.pressure)
		}
		delete(r.parked, ev.Channel)
	}

	r.active[ev.Channel] = note
}

// noteOff closes the note matched by member channel and note number. A
// mismatched note-off (wrong note, or no active note on the channel) is
// counted as an orphan and never tears down an unrelated note.
func (r *Recorder) noteOff(ev Event) {
	if r.isMaster(ev.Channel) {
		r.masterNoteOff(ev)
		return
	}
	note, ok := r.active[ev.Channel]
	if !ok {
		r.Orphans++
		return
	}
	if note.Note != ev.Note {
		r.Orphans++
		return
	}
	note.State = StateClosed
	note.EndTick = ev.Tick
	note.OffVelocity = ev.Velocity
	r.finished = append(r.finished, note)
	delete(r.active, ev.Channel)
}

// masterNoteOff closes the matching master-channel note. Master notes are
// keyed by synthetic slots, so they are found by scanning for the note
// number rather than by channel alone.
func (r *Recorder) masterNoteOff(ev Event) {
	slot := masterSlot(ev.Note)
	note, ok := r.active[slot]
	if !ok || note.Channel != ev.Channel {
		r.Orphans++
		return
	}
	note.State = StateClosed
	note.EndTick = ev.Tick
	note.OffVelocity = ev.Velocity
	r.finished = append(r.finished, note)
	delete(r.active, slot)
}

// startMasterNote handles notes on a zone master channel. Multiple
// simultaneous master notes are legal in conventional MIDI; they share the
// master channel but receive distinct note IDs.
func (r *Recorder) startMasterNote(ev Event) {
	note := &Note{
		ID:        r.nextID,
		Channel:   ev.Channel,
		Note:      ev.Note,
		StartTick: ev.Tick,
		Velocity:  ev.Velocity,
		State:     StateActive,
	}
	r.nextID++
	// Master notes are kept in active keyed by a synthetic slot so they do
	// not collide with member channels (0..15).
	r.active[masterSlot(ev.Note)] = note
}

// masterSlot maps a master-channel note number to a map slot outside the
// real channel range.
func masterSlot(note int) int {
	return 16 + note
}

// allNotesOff flushes active notes. When ch is a master channel the whole
// zone is affected; otherwise only notes on that member channel.
func (r *Recorder) allNotesOff(ch int, tick int64, state NoteState) {
	zone := r.zoneForMaster(ch)

	for slot, note := range r.active {
		match := false
		switch {
		case zone != nil:
			match = note.Channel == zone.Master || zone.contains(note.Channel)
		default:
			match = slot == ch
		}
		if !match {
			continue
		}
		note.State = state
		note.EndTick = tick
		r.finished = append(r.finished, note)
		delete(r.active, slot)
	}
}

// Flush closes every still-active note (recording ended without a
// note-off) and returns the complete ordered list of notes. After Flush
// the recorder has no active notes but can be reused.
func (r *Recorder) Flush(tick int64) []*Note {
	for slot, note := range r.active {
		note.State = StateFlushed
		if tick > note.EndTick {
			note.EndTick = tick
		}
		r.finished = append(r.finished, note)
		delete(r.active, slot)
	}
	out := r.finished
	r.finished = nil
	r.parked = map[int]*parked{}
	return out
}

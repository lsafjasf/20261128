package mpe

import "testing"

func noteOn(tick int64, ch, note, vel int) Event {
	return Event{Tick: tick, Channel: ch, Kind: KindNoteOn, Note: note, Velocity: vel}
}
func noteOff(tick int64, ch, note, vel int) Event {
	return Event{Tick: tick, Channel: ch, Kind: KindNoteOff, Note: note, Velocity: vel}
}
func bend(tick int64, ch, v int) Event {
	return Event{Tick: tick, Channel: ch, Kind: KindPitchBend, Value: v}
}
func pressure(tick int64, ch, v int) Event {
	return Event{Tick: tick, Channel: ch, Kind: KindChannelPressure, Value: v}
}

// The core MPE scenario: one member channel is reused for several distinct
// notes. Each must get its own note ID and its own expression stream.
func TestChannelReuseGivesDistinctNotes(t *testing.T) {
	r := NewRecorder()
	ch := 1 // member channel in the default lower zone

	r.Process(noteOn(0, ch, 60, 100))
	r.Process(bend(1, ch, 9000))
	r.Process(noteOff(2, ch, 60, 80))

	r.Process(noteOn(3, ch, 64, 110))
	r.Process(bend(4, ch, 7000))
	r.Process(noteOff(5, ch, 64, 90))

	notes := r.Flush(5)
	if len(notes) != 2 {
		t.Fatalf("want 2 notes, got %d", len(notes))
	}
	if notes[0].ID == notes[1].ID {
		t.Fatal("reused channel must produce distinct note IDs")
	}
	if notes[0].Note != 60 || notes[1].Note != 64 {
		t.Fatal("note numbers not reconstructed correctly")
	}
	if len(notes[0].Bend) != 1 || notes[0].Bend[0].Value != 9000 {
		t.Fatalf("note 0 bend lost: %+v", notes[0].Bend)
	}
	if len(notes[1].Bend) != 1 || notes[1].Bend[0].Value != 7000 {
		t.Fatalf("note 1 bend leaked across notes: %+v", notes[1].Bend)
	}
	if notes[0].State != StateClosed || notes[0].OffVelocity != 80 {
		t.Fatal("note 0 not closed with off velocity")
	}
}

// Expression arriving before note-on (typical MPE gesture) is parked on the
// member channel and inherited by the next note, not dropped.
func TestParkedExpressionInherited(t *testing.T) {
	r := NewRecorder()
	ch := 2

	r.Process(bend(0, ch, 8500))
	r.Process(pressure(1, ch, 64))
	r.Process(noteOn(2, ch, 72, 90))
	r.Process(noteOff(3, ch, 72, 0))

	notes := r.Flush(3)
	if len(notes) != 1 {
		t.Fatalf("want 1 note, got %d", len(notes))
	}
	n := notes[0]
	if len(n.Bend) != 1 || n.Bend[0].Value != 8500 {
		t.Fatalf("parked bend not inherited: %+v", n.Bend)
	}
	if len(n.Pressure) != 1 || n.Pressure[0].Value != 64 {
		t.Fatalf("parked pressure not inherited: %+v", n.Pressure)
	}
}

// Channel exhaustion: a second note-on arrives on a channel whose previous
// note is still active. The previous note must be preserved as stolen.
func TestChannelExhaustionStealsWithoutLoss(t *testing.T) {
	r := NewRecorder()
	ch := 1

	r.Process(noteOn(0, ch, 60, 100))
	r.Process(bend(1, ch, 9000))
	// No note-off; controller recycles the channel.
	r.Process(noteOn(2, ch, 67, 110))
	r.Process(noteOff(3, ch, 67, 50))

	notes := r.Flush(3)
	if len(notes) != 2 {
		t.Fatalf("stolen note must not be lost, got %d notes", len(notes))
	}
	first, second := notes[0], notes[1]
	if first.State != StateStolen {
		t.Fatalf("want StateStolen, got %v", first.State)
	}
	if first.Note != 60 || len(first.Bend) != 1 {
		t.Fatal("stolen note data not preserved")
	}
	if second.Note != 67 || second.State != StateClosed {
		t.Fatal("replacement note reconstructed incorrectly")
	}
	if r.Stolen != 1 {
		t.Fatalf("want Stolen==1, got %d", r.Stolen)
	}
}

// Out of order timestamps must not corrupt monotonic ordering, and a stray
// note-off must never kill an unrelated note.
func TestOutOfOrderAndOrphans(t *testing.T) {
	r := NewRecorder()
	ch := 1

	// Note-off with no matching note: orphan, no panic, no note lost.
	r.Process(noteOff(0, ch, 60, 0))
	if r.Orphans != 1 {
		t.Fatalf("want 1 orphan, got %d", r.Orphans)
	}

	r.Process(noteOn(10, ch, 60, 100))
	// Late timestamp: clamped to the last tick rather than rewinding.
	r.Process(bend(5, ch, 8192))
	r.Process(noteOff(12, ch, 60, 0))

	// Note-off for the wrong note: orphan, real note survives.
	r.Process(noteOn(13, ch, 62, 100))
	r.Process(noteOff(14, ch, 99, 0))

	notes := r.Flush(20)
	if len(notes) != 2 {
		t.Fatalf("want 2 notes despite bad messages, got %d", len(notes))
	}
	if notes[0].Bend[0].Tick < notes[0].StartTick {
		t.Fatal("out of order bend tick was not clamped")
	}
	if r.Orphans != 2 {
		t.Fatalf("want 2 orphans, got %d", r.Orphans)
	}
}

// Flush closes notes that never received a note-off (e.g. recording
// stopped mid-gesture) so they are still emitted.
func TestFlushClosesActiveNotes(t *testing.T) {
	r := NewRecorder()
	r.Process(noteOn(0, 1, 60, 100))
	r.Process(noteOn(0, 2, 64, 100))

	if len(r.ActiveNotes()) != 2 {
		t.Fatal("expected 2 active notes")
	}
	notes := r.Flush(7)
	if len(notes) != 2 {
		t.Fatalf("flush must emit active notes, got %d", len(notes))
	}
	for _, n := range notes {
		if n.State != StateFlushed || n.EndTick != 7 {
			t.Fatalf("note not flushed correctly: %+v", n)
		}
	}
}

// Many concurrent notes spread across all member channels, then released,
// all survive with independent expression.
func TestManyConcurrentNotes(t *testing.T) {
	r := NewRecorder()
	for ch := 1; ch <= 15; ch++ {
		r.Process(noteOn(0, ch, 40+ch, 90))
		r.Process(bend(1, ch, 8192+ch))
	}
	for ch := 1; ch <= 15; ch++ {
		r.Process(noteOff(2, ch, 40+ch, 0))
	}
	notes := r.Flush(2)
	if len(notes) != 15 {
		t.Fatalf("want 15 notes, got %d", len(notes))
	}
	ids := map[int64]bool{}
	for _, n := range notes {
		if ids[n.ID] {
			t.Fatal("duplicate note ID")
		}
		ids[n.ID] = true
		if len(n.Bend) != 1 {
			t.Fatalf("note on ch %d lost bend", n.Channel)
		}
	}
}

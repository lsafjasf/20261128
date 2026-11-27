package mpe

import "testing"

func TestPolyPressureMatchesOnlyOwnNote(t *testing.T) {
	r := NewRecorder()
	r.Process(noteOn(0, 1, 60, 100))
	// Poly pressure for a different key on the same channel is ignored.
	r.Process(Event{Tick: 1, Channel: 1, Kind: KindPolyPressure, Note: 61, Value: 70})
	r.Process(Event{Tick: 2, Channel: 1, Kind: KindPolyPressure, Note: 60, Value: 90})
	r.Process(noteOff(3, 1, 60, 0))

	n := r.Flush(3)[0]
	if len(n.Pressure) != 1 || !n.Pressure[0].Poly || n.Pressure[0].Value != 90 {
		t.Fatalf("poly pressure routing wrong: %+v", n.Pressure)
	}
}

func TestCC74TimbreRecorded(t *testing.T) {
	r := NewRecorder()
	r.Process(Event{Tick: 0, Channel: 1, Kind: KindControlChange, CC: 74, Value: 33})
	r.Process(noteOn(1, 1, 60, 100))
	r.Process(Event{Tick: 2, Channel: 1, Kind: KindControlChange, CC: 74, Value: 44})
	r.Process(noteOff(3, 1, 60, 0))

	n := r.Flush(3)[0]
	if len(n.Bend) != 2 || n.Bend[0].Value != 33 || n.Bend[1].Value != 44 {
		t.Fatalf("CC74 not captured/parked: %+v", n.Bend)
	}
}

func TestParkedDataConsumedOnce(t *testing.T) {
	r := NewRecorder()
	r.Process(bend(0, 1, 9000))
	r.Process(noteOn(1, 1, 60, 100))
	r.Process(noteOff(2, 1, 60, 0))
	// Second note on the same channel must not inherit the first gesture.
	r.Process(noteOn(3, 1, 64, 100))
	r.Process(noteOff(4, 1, 64, 0))

	notes := r.Flush(4)
	if len(notes[1].Bend) != 0 {
		t.Fatalf("parked bend leaked into later note: %+v", notes[1].Bend)
	}
}

func TestUnknownAndSystemMessagesDoNotCrash(t *testing.T) {
	r := NewRecorder()
	r.Process(Event{Tick: 0, Kind: KindUnknown}) // counted as orphan
	rd := NewReader()
	ev := rd.Parse(1, []byte{0xF0, 0x01, 0xF7}) // SysEx
	if ev.Kind != KindUnknown {
		t.Fatal("system message should parse to unknown")
	}
	r.Feed(rd, 1, []byte{0x91, 60, 100}) // exercises Feed
	r.Feed(rd, 2, []byte{0xF8})          // realtime clock, ignored

	notes := r.Flush(2)
	if len(notes) != 1 {
		t.Fatalf("note lost among system messages: %+v", notes)
	}
	if r.Orphans != 1 {
		t.Fatalf("want 1 orphan for unknown event, got %d", r.Orphans)
	}
}

func TestNotesAccessorReturnsClosedOnly(t *testing.T) {
	r := NewRecorder()
	r.Process(noteOn(0, 1, 60, 100))
	r.Process(noteOff(1, 1, 60, 0))
	r.Process(noteOn(2, 2, 64, 100)) // still active

	closed := r.Notes()
	if len(closed) != 1 || closed[0].Note != 60 {
		t.Fatalf("Notes() should expose only finished notes: %+v", closed)
	}
}

func TestUpperAllNotesOff(t *testing.T) {
	r := NewZonedRecorder(0, 2)
	r.Process(noteOn(0, 13, 50, 100))
	r.Process(noteOn(0, 14, 52, 100))
	r.Process(Event{Tick: 1, Channel: 15, Kind: KindAllNotesOff})

	if len(r.Flush(1)) != 2 {
		t.Fatal("upper all-notes-off must flush upper members")
	}
}

func TestRPNNonConfigIgnored(t *testing.T) {
	r := NewRecorder()
	cc := func(ccNum, val int) Event {
		return Event{Kind: KindControlChange, Channel: 0, CC: ccNum, Value: val}
	}
	// RPN 0 (pitch bend sensitivity), not RPN 6: zone must stay default.
	r.Process(cc(101, 0))
	r.Process(cc(100, 0))
	r.Process(cc(6, 12))
	if len(r.lower.Members) != 15 {
		t.Fatal("non-MPE RPN must not reconfigure zones")
	}
}

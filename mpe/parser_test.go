package mpe

import "testing"

func TestParseBasicAndRunningStatus(t *testing.T) {
	rd := NewReader()

	on := rd.Parse(0, []byte{0x91, 60, 100})
	if on.Kind != KindNoteOn || on.Channel != 1 || on.Note != 60 || on.Velocity != 100 {
		t.Fatalf("bad note-on: %+v", on)
	}

	// Running status: status byte omitted.
	on2 := rd.Parse(1, []byte{64, 110})
	if on2.Kind != KindNoteOn || on2.Channel != 1 || on2.Note != 64 {
		t.Fatalf("running status failed: %+v", on2)
	}

	// Pitch bend is little-endian 14 bit.
	pb := rd.Parse(2, []byte{0xE1, 0x00, 0x40})
	if pb.Kind != KindPitchBend || pb.Value != 8192 {
		t.Fatalf("bend decode failed, got %d", pb.Value)
	}

	off := rd.Parse(3, []byte{0x81, 60, 0})
	if off.Kind != KindNoteOff || off.Note != 60 {
		t.Fatalf("bad note-off: %+v", off)
	}
}

func TestVelocityZeroNoteOnActsAsOff(t *testing.T) {
	r := NewRecorder()
	r.Process(noteOn(0, 1, 60, 100))
	r.Process(Event{Tick: 1, Channel: 1, Kind: KindNoteOn, Note: 60, Velocity: 0})

	notes := r.Flush(1)
	if len(notes) != 1 || notes[0].State != StateClosed {
		t.Fatalf("velocity-0 note-on should close note: %+v", notes)
	}
}

func TestMPEConfigReconfiguresZones(t *testing.T) {
	r := NewRecorder()
	// Lower zone RPN 6 -> 7 member channels on master ch 0.
	cc := func(ccNum, val int) Event {
		return Event{Kind: KindControlChange, Channel: 0, CC: ccNum, Value: val}
	}
	r.Process(cc(101, 0))
	r.Process(cc(100, 6))
	r.Process(cc(6, 7))

	if len(r.lower.Members) != 7 || r.lower.Members[0] != 1 || r.lower.Members[6] != 7 {
		t.Fatalf("lower zone not configured: %+v", r.lower.Members)
	}
	// Channel 8 is no longer a member.
	if r.classify(8) != nil {
		t.Fatal("channel 8 should be outside the configured lower zone")
	}
	if r.classify(5) == nil {
		t.Fatal("channel 5 should be a member")
	}
}

func TestUpperZone(t *testing.T) {
	r := NewZonedRecorder(0, 2)
	if r.classify(13) == nil || r.classify(14) == nil {
		t.Fatal("upper members 13,14 should be active")
	}
	if r.isMaster(15) != true {
		t.Fatal("ch 15 should be upper master")
	}

	r.Process(noteOn(0, 13, 55, 100))
	r.Process(bend(1, 13, 9000))
	r.Process(noteOff(2, 13, 55, 0))
	notes := r.Flush(2)
	if len(notes) != 1 || len(notes[0].Bend) != 1 {
		t.Fatalf("upper zone note failed: %+v", notes)
	}
}

func TestMasterChannelNotesAndFanOut(t *testing.T) {
	r := NewZonedRecorder(7, 0)
	// Two simultaneous notes on the master channel get distinct IDs.
	r.Process(noteOn(0, 0, 60, 100))
	r.Process(noteOn(0, 0, 72, 100))
	// Master bend fans out to both.
	r.Process(bend(1, 0, 9000))
	r.Process(noteOff(2, 0, 60, 0))
	r.Process(noteOff(3, 0, 72, 0))

	notes := r.Flush(3)
	if len(notes) != 2 {
		t.Fatalf("want 2 master notes, got %d", len(notes))
	}
	for _, n := range notes {
		if len(n.Bend) != 1 || n.Bend[0].Value != 9000 {
			t.Fatalf("master bend did not fan out: %+v", n.Bend)
		}
	}
}

func TestAllNotesOff(t *testing.T) {
	r := NewRecorder()
	r.Process(noteOn(0, 1, 60, 100))
	r.Process(noteOn(0, 2, 64, 100))
	r.Process(Event{Tick: 1, Channel: 0, Kind: KindAllNotesOff})

	notes := r.Flush(1)
	if len(notes) != 2 {
		t.Fatalf("all-notes-off should flush both notes, got %d", len(notes))
	}
	for _, n := range notes {
		if n.State != StateFlushed {
			t.Fatalf("want flushed, got %v", n.State)
		}
	}
}

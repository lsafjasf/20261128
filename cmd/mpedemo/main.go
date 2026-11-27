// Command mpedemo feeds a synthetic MPE byte stream through the recorder
// and prints the reconstructed note lifecycles. It demonstrates the three
// hard cases: channel recycling, channel exhaustion (voice stealing), and
// bend/pressure that arrives before the note-on.
package main

import (
	"fmt"

	"mpe-recorder/mpe"
)

func main() {
	rec := mpe.NewRecorder()
	rd := mpe.NewReader()
	var tick int64

	// Each entry is a raw MIDI message; tick advances per message.
	stream := [][]byte{
		// Note A on member channel 2: gesture starts with bend+pressure.
		{0xE1, 0x28, 0x42}, // pitch bend ch2
		{0xD1, 0x50},       // channel pressure ch2
		{0x91, 60, 100},    // note-on ch2, note 60
		{0xE1, 0x40, 0x43}, // bend while held
		{0x81, 60, 80},     // note-off ch2

		// Note B reuses the same member channel -> must be a new note ID.
		{0x91, 64, 110}, // note-on ch2, note 64
		{0xE1, 0x00, 0x3F},
		{0x81, 64, 90}, // note-off

		// Channel exhaustion: note C starts on ch3 and never gets an
		// explicit off before ch3 is reused for note D.
		{0x92, 48, 95}, // note-on ch3, note 48
		{0xA2, 48, 77}, // poly pressure for note 48
		{0x92, 55, 95}, // reuse ch3 -> note 48 is stolen, note 55 starts
		{0x82, 55, 60}, // note-off note 55

		// Note E left ringing; flush must still emit it.
		{0x93, 72, 100}, // note-on ch4
		{0xE3, 0x10, 0x41},
	}

	for _, msg := range stream {
		tick++
		rec.Feed(rd, tick, msg)
	}

	notes := rec.Flush(tick)
	fmt.Printf("reconstructed %d notes (stolen=%d orphans=%d)\n\n",
		len(notes), rec.Stolen, rec.Orphans)

	for _, n := range notes {
		fmt.Printf("noteID=%d ch=%d note=%d state=%s tick[%d..%d] vel=%d offVel=%d\n",
			n.ID, n.Channel+1, n.Note, stateName(n.State),
			n.StartTick, n.EndTick, n.Velocity, n.OffVelocity)
		for _, b := range n.Bend {
			fmt.Printf("    bend  tick=%d value=%d\n", b.Tick, b.Value)
		}
		for _, p := range n.Pressure {
			kind := "channel"
			if p.Poly {
				kind = "poly"
			}
			fmt.Printf("    %s pressure tick=%d value=%d\n", kind, p.Tick, p.Value)
		}
	}
}

func stateName(s mpe.NoteState) string {
	switch s {
	case mpe.StateClosed:
		return "closed"
	case mpe.StateStolen:
		return "stolen"
	case mpe.StateFlushed:
		return "flushed"
	default:
		return "active"
	}
}

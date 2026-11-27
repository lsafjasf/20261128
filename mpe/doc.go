// Package mpe reconstructs MIDI Polyphonic Expression (MPE) note
// lifecycles from a raw channel-message stream.
//
// An MPE controller does not give each note a fixed channel. Instead it
// borrows a member channel for the duration of a gesture and returns it to
// a pool when the key lifts. A naive recorder that keys notes by channel
// therefore merges unrelated gestures into phantom "voices" and loses
// expression whenever a channel is reused.
//
// This package solves that by assigning every note-on a monotonic note ID
// and tracking the (note ID, member channel) pair for the whole gesture.
// Pitch bend, channel pressure, polyphonic pressure and the MPE CC74
// "timbre" axis are attached to whichever note currently owns the member
// channel, so each note keeps its own complete expression history.
//
// # Robustness rules
//
// Expression that arrives before the note-on (a normal part of an MPE
// gesture) is parked on the member channel and inherited by the next note,
// rather than discarded.
//
// When a note-on arrives on a member channel whose previous note never
// received a note-off (channel exhaustion), the previous note is closed
// with StateStolen and fully preserved; the new note still starts normally.
// No note is overwritten or dropped.
//
// A note-off that matches no active note, or the wrong note number, is
// counted in Recorder.Orphans and never tears down an unrelated note.
// Timestamps that move backwards are clamped so ordering stays monotonic.
// Notes still active when recording ends are emitted by Flush with
// StateFlushed.
//
// # Zones
//
// The factory default is a lower zone: master channel 1 (zero based 0)
// with the other 15 channels as members. MPE Configuration Messages
// (RPN 6) reconfigure the lower and upper zones; see NewZonedRecorder.
//
// # Usage
//
// Feed either parsed Events through Recorder.Process or raw bytes through
// Recorder.Feed with a Reader, then call Flush to obtain the notes:
//
//	rec := mpe.NewRecorder()
//	rd := mpe.NewReader()
//	for tick, msg := range stream {
//		rec.Feed(rd, int64(tick), msg)
//	}
//	notes := rec.Flush(lastTick)
//
// A Recorder is single-goroutine; guard it with a channel or mutex if the
// source is concurrent.
package mpe

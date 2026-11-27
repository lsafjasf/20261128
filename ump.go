// Package ump parses Universal MIDI Packet (UMP) streams into lossless,
// packet-bounded events.
package ump

import "fmt"

// MessageType is the four-bit UMP Message Type field.
type MessageType uint8

const (
	MTUtility           MessageType = 0x0
	MTSystem            MessageType = 0x1
	MTMIDI1ChannelVoice MessageType = 0x2
	MTData64            MessageType = 0x3
	MTMIDI2ChannelVoice MessageType = 0x4
	MTData128           MessageType = 0x5
	MTReserved6         MessageType = 0x6
	MTReserved7         MessageType = 0x7
	MTReserved8         MessageType = 0x8
	MTReserved9         MessageType = 0x9
	MTReservedA         MessageType = 0xA
	MTFlexData          MessageType = 0xB
	MTReservedC         MessageType = 0xC
	MTReservedD         MessageType = 0xD
	MTReservedE         MessageType = 0xE
	MTUMPStream         MessageType = 0xF
)

// packetWords is the normative UMP packet size in 32-bit words, indexed by
// Message Type. The values for reserved types matter: they keep the parser in
// sync even when a message is otherwise unknown.
var packetWords = [16]uint8{
	1, 1, 1, 2, 2, 4, 1, 1, 2, 2, 2, 3, 3, 4, 4, 4,
}

// Words returns the packet size in 32-bit words.
func (mt MessageType) Words() int {
	return int(packetWords[mt&0x0F])
}

// Bytes returns the packet size in bytes.
func (mt MessageType) Bytes() int {
	return mt.Words() * 4
}

func (mt MessageType) String() string {
	switch mt & 0x0F {
	case MTUtility:
		return "utility"
	case MTSystem:
		return "system"
	case MTMIDI1ChannelVoice:
		return "midi1-channel-voice"
	case MTData64:
		return "data64"
	case MTMIDI2ChannelVoice:
		return "midi2-channel-voice"
	case MTData128:
		return "data128"
	case MTFlexData:
		return "flex-data"
	case MTUMPStream:
		return "ump-stream"
	default:
		return fmt.Sprintf("reserved-0x%X", uint8(mt&0x0F))
	}
}

// Kind is the decoded message family. It is deliberately independent from
// MessageType: an unknown status inside a known Message Type still has a
// reliable UMP boundary.
type Kind uint8

const (
	KindUnknown Kind = iota
	KindNoteOff
	KindNoteOn
	KindPolyPressure
	KindControlChange
	KindProgramChange
	KindChannelPressure
	KindPitchBend
	KindPerNoteRegisteredController
	KindPerNoteAssignableController
	KindRegisteredController
	KindAssignableController
	KindRelativeRegisteredController
	KindRelativeAssignableController
	KindPerNotePitchBend
	KindPerNoteManagement
	KindSysEx7
	KindSysEx8
	KindMixedDataSet
	KindJRClock
	KindJRTimestamp
	KindDeltaClockstampTPQ
	KindDeltaClockstamp
	KindSystem
	KindFlexData
	KindUMPStream
	KindGroupEnd
)

// Word is a UMP word together with the transport timestamp for that word.
// Timestamps are caller-defined transport units (for example nanoseconds);
// the parser never scales or rewrites them.
type Word struct {
	Value     uint32
	Timestamp uint64
}

// Event is one complete UMP packet. Words is always a copy of the exact packet
// words, including for unknown and truncated messages. Timestamp is the
// timestamp of the first word. Group is the UMP Group from the first word;
// UMP Stream messages are groupless and therefore use Group zero.
type Event struct {
	Timestamp uint64
	Group     uint8
	Type      MessageType
	Status    uint16
	Words     []uint32

	Known     bool
	Truncated bool
	Kind      Kind

	Channel       uint8
	Note          uint8
	Controller    uint8
	Program       uint8
	Bank          uint8
	BankMSB       uint8
	BankLSB       uint8
	BankValid     bool
	Index         uint8
	AttributeType uint8
	AttributeData uint16
	Velocity      uint32
	Value         uint32
	Data          []byte
}

// Options controls event synthesis.
type Options struct {
	// EmitGroupEnd adds a KindGroupEnd event whenever the UMP Group changes and
	// once at the end of a non-empty stream. Its timestamp is the timestamp of
	// the last word in the group.
	EmitGroupEnd bool
}

// TruncatedError reports the final packet that did not contain all words
// required by its Message Type. The partial packet is still returned as an
// Event with Truncated set, so callers can preserve or inspect it.
type TruncatedError struct {
	WordOffset int
	Have       int
	Want       int
	Type       MessageType
}

func (e *TruncatedError) Error() string {
	return fmt.Sprintf(
		"ump: truncated %s packet at word %d: have %d word(s), want %d",
		e.Type, e.WordOffset, e.Have, e.Want,
	)
}

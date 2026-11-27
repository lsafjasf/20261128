package ump

import (
	"encoding/binary"
	"fmt"
)

// Parse splits untimed UMP words at Message Type boundaries. Every event keeps
// its exact source words, including reserved Message Types and unknown statuses.
func Parse(words []uint32) ([]Event, error) {
	return ParseWithOptions(words, Options{})
}

// ParseWithOptions is Parse with event synthesis options.
func ParseWithOptions(words []uint32, opts Options) ([]Event, error) {
	timed := make([]Word, len(words))
	for i, word := range words {
		timed[i].Value = word
	}
	return ParseTimedWithOptions(timed, opts)
}

// ParseTimed splits words at Message Type boundaries and carries transport
// timestamps. For a multi-word UMP packet, the event timestamp comes from the
// first word and all words remain available in Event.Words.
func ParseTimed(words []Word) ([]Event, error) {
	return ParseTimedWithOptions(words, Options{})
}

// ParseTimedWithOptions is ParseTimed with event synthesis options.
func ParseTimedWithOptions(words []Word, opts Options) ([]Event, error) {
	var (
		events        []Event
		currentGroup  uint8
		haveGroup     bool
		groupEndStamp uint64
		resultErr     error
	)

	for offset := 0; offset < len(words); {
		mt := MessageType(words[offset].Value >> 28)
		want := mt.Words()
		have := min(want, len(words)-offset)

		raw := make([]uint32, have)
		for i := range raw {
			raw[i] = words[offset+i].Value
		}
		ev := decodeEvent(raw, words[offset].Timestamp)
		if have < want {
			ev.Truncated = true
			ev.Known = false
			ev.Kind = KindUnknown
		}

		if opts.EmitGroupEnd && haveGroup && mtHasGroup(ev.Type) && ev.Group != currentGroup {
			events = append(events, makeGroupEnd(currentGroup, groupEndStamp))
			haveGroup = false
		}

		events = append(events, ev)
		if mtHasGroup(ev.Type) {
			currentGroup = ev.Group
			haveGroup = true
			groupEndStamp = words[offset+have-1].Timestamp
		}

		offset += have
		if have < want {
			resultErr = &TruncatedError{
				WordOffset: offset - have,
				Have:       have,
				Want:       want,
				Type:       mt,
			}
			break
		}
	}

	if opts.EmitGroupEnd && haveGroup {
		events = append(events, makeGroupEnd(currentGroup, groupEndStamp))
	}
	return events, resultErr
}

func mtHasGroup(mt MessageType) bool {
	return mt != MTUMPStream
}

func makeGroupEnd(group uint8, timestamp uint64) Event {
	return Event{
		Timestamp: timestamp,
		Group:     group,
		Known:     true,
		Kind:      KindGroupEnd,
	}
}

// ParseBytes parses big-endian, byte-oriented UMP data. UMP words are encoded
// most-significant byte first on byte-oriented transports.
func ParseBytes(data []byte) ([]Event, error) {
	return ParseBytesOrder(data, binary.BigEndian)
}

// ParseBytesOrder parses byte-oriented UMP data with an explicit word byte
// order. A trailing partial word is rejected before any events are returned.
func ParseBytesOrder(data []byte, order binary.ByteOrder) ([]Event, error) {
	if len(data)%4 != 0 {
		return nil, fmt.Errorf("ump: byte input length %d is not a multiple of 4", len(data))
	}
	words := make([]uint32, len(data)/4)
	for i := range words {
		words[i] = order.Uint32(data[i*4:])
	}
	return Parse(words)
}

// Flatten concatenates the original words of all events. Synthetic group-end
// events contribute no words, so Flatten(Parse(words)) is a lossless round trip
// for a complete stream, including unknown messages.
func Flatten(events []Event) []uint32 {
	var size int
	for _, ev := range events {
		size += len(ev.Words)
	}
	words := make([]uint32, 0, size)
	for _, ev := range events {
		words = append(words, ev.Words...)
	}
	return words
}

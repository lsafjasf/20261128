package ump

import (
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
)

func TestMessageTypeWords(t *testing.T) {
	t.Parallel()
	want := [16]int{1, 1, 1, 2, 2, 4, 1, 1, 2, 2, 2, 3, 3, 4, 4, 4}
	for mt := MessageType(0); mt < 16; mt++ {
		if got := mt.Words(); got != want[mt] {
			t.Errorf("MessageType(0x%X).Words() = %d, want %d", mt, got, want[mt])
		}
		if got := mt.Bytes(); got != want[mt]*4 {
			t.Errorf("MessageType(0x%X).Bytes() = %d, want %d", mt, got, want[mt]*4)
		}
	}
}

func TestMixedStreamUsesUMPBoundaries(t *testing.T) {
	t.Parallel()
	words := []uint32{
		midi1NoteOn(2, 3, 60, 127),
		midi2NoteOn(2, 3, 60, 0x01),
		0xABCD0000,
		perNoteController(2, 3, 61, 74),
		0xFEDCBA98,
		unknownMIDI2(2, 0x7),
		0xA5A5A5A5,
		rawHeader(MTReserved9, 1, 0x5, 0x3),
		0xDEADBEEF,
		rawHeader(MTData128, 1, 0x1, 0x3),
		0x01020304,
		0x05060708,
		0x090A0B0C,
		rawHeader(MTFlexData, 1, 0x0, 0x0),
		0x11121314,
		0x15161718,
		rawHeader(MTUMPStream, 0, 0x2, 0x0),
		0x21222324,
		0x25262728,
		0x292A2B2C,
	}

	events, err := Parse(words)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	wantSizes := []int{1, 2, 2, 2, 2, 4, 3, 4}
	if len(events) != len(wantSizes) {
		for i, ev := range events {
			t.Logf("event %d: type=%s words=%d raw=%#v", i, ev.Type, len(ev.Words), ev.Words)
		}
		t.Fatalf("got %d events, want %d", len(events), len(wantSizes))
	}

	at := 0
	for i, wantSize := range wantSizes {
		ev := events[i]
		if len(ev.Words) != wantSize {
			t.Fatalf("event %d (%s) has %d words, want %d", i, ev.Type, len(ev.Words), wantSize)
		}
		if got := ev.Words; !reflect.DeepEqual(got, words[at:at+wantSize]) {
			t.Fatalf("event %d words = %#v, want %#v", i, got, words[at:at+wantSize])
		}
		at += wantSize
	}
	if at != len(words) {
		t.Fatalf("parsed %d words, stream has %d", at, len(words))
	}
	if got := Flatten(events); !reflect.DeepEqual(got, words) {
		t.Errorf("Flatten(Parse(words)) lost or reordered data")
	}

	if events[0].Type != MTMIDI1ChannelVoice || events[0].Velocity != 127 {
		t.Errorf("MIDI 1.0 compatibility event = %+v", events[0])
	}
	if events[1].Type != MTMIDI2ChannelVoice || events[1].Velocity != 0xABCD {
		t.Errorf("MIDI 2.0 16-bit velocity event = %+v", events[1])
	}
	if events[2].Kind != KindPerNoteRegisteredController ||
		events[2].Note != 61 ||
		events[2].Controller != 74 ||
		events[2].Value != 0xFEDCBA98 {
		t.Errorf("per-note controller event = %+v", events[2])
	}
	if events[3].Known || events[3].Kind != KindUnknown {
		t.Errorf("unknown MIDI 2.0 status should be preserved as unknown: %+v", events[3])
	}
	if events[4].Type != MTReserved9 || events[4].Known {
		t.Errorf("reserved message type should be preserved as unknown: %+v", events[4])
	}
	if events[7].Type != MTUMPStream || events[7].Group != 0 {
		t.Errorf("UMP Stream event = %+v", events[7])
	}
}

func TestPerNoteControllersAndAttributes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		word uint32
		data uint32
		want Event
	}{
		{
			name: "registered per-note controller",
			word: perNoteController(5, 4, 64, 7),
			data: 0x10203040,
			want: Event{
				Kind: KindPerNoteRegisteredController, Channel: 4, Note: 64,
				Controller: 7, Index: 7, Value: 0x10203040,
			},
		},
		{
			name: "assignable per-note controller",
			word: rawHeader(MTMIDI2ChannelVoice, 5, 0x1, 4) | 64<<8 | 9,
			data: 0x50607080,
			want: Event{
				Kind: KindPerNoteAssignableController, Channel: 4, Note: 64,
				Controller: 9, Index: 9, Value: 0x50607080,
			},
		},
		{
			name: "note on attribute",
			word: rawHeader(MTMIDI2ChannelVoice, 5, 0x9, 4) | 65<<8 | 0x03,
			data: 0xABCD1234,
			want: Event{
				Kind: KindNoteOn, Channel: 4, Note: 65, AttributeType: 0x03,
				Velocity: 0xABCD, AttributeData: 0x1234,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			events, err := Parse([]uint32{tt.word, tt.data})
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1", len(events))
			}
			got := events[0]
			if !got.Known {
				t.Fatalf("event unexpectedly unknown: %+v", got)
			}
			if got.Kind != tt.want.Kind ||
				got.Channel != tt.want.Channel ||
				got.Note != tt.want.Note ||
				got.Controller != tt.want.Controller ||
				got.Index != tt.want.Index ||
				got.Velocity != tt.want.Velocity ||
				got.Value != tt.want.Value ||
				got.AttributeType != tt.want.AttributeType ||
				got.AttributeData != tt.want.AttributeData {
				t.Errorf("decoded event = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTimestampsAndGroupEnd(t *testing.T) {
	t.Parallel()
	words := []Word{
		{Value: midi1NoteOn(0, 0, 60, 1), Timestamp: 100},
		{Value: midi2NoteOn(0, 0, 61, 0), Timestamp: 110},
		{Value: 0, Timestamp: 111},
		{Value: midi1NoteOn(1, 0, 62, 3), Timestamp: 200},
		{Value: midi1NoteOn(2, 0, 63, 4), Timestamp: 300},
	}

	events, err := ParseTimedWithOptions(words, Options{EmitGroupEnd: true})
	if err != nil {
		t.Fatalf("ParseTimedWithOptions() error = %v", err)
	}
	want := []struct {
		kind      Kind
		group     uint8
		timestamp uint64
	}{
		{KindNoteOn, 0, 100},
		{KindNoteOn, 0, 110},
		{KindGroupEnd, 0, 111},
		{KindNoteOn, 1, 200},
		{KindGroupEnd, 1, 200},
		{KindNoteOn, 2, 300},
		{KindGroupEnd, 2, 300},
	}
	if len(events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(events), len(want), events)
	}
	for i, expected := range want {
		if events[i].Kind != expected.kind ||
			events[i].Group != expected.group ||
			events[i].Timestamp != expected.timestamp {
			t.Errorf("event %d = {kind:%d group:%d timestamp:%d}, want %+v",
				i, events[i].Kind, events[i].Group, events[i].Timestamp, expected)
		}
	}
}

func TestTruncatedPacketIsPreserved(t *testing.T) {
	t.Parallel()
	partial := rawHeader(MTMIDI2ChannelVoice, 4, 0x9, 2) | 60<<8 | 1
	words := []Word{
		{Value: midi1NoteOn(4, 2, 59, 100), Timestamp: 10},
		{Value: partial, Timestamp: 20},
	}

	events, err := ParseTimed(words)
	var truncated *TruncatedError
	if !errors.As(err, &truncated) {
		t.Fatalf("error = %v, want *TruncatedError", err)
	}
	if truncated.WordOffset != 1 || truncated.Have != 1 || truncated.Want != 2 ||
		truncated.Type != MTMIDI2ChannelVoice {
		t.Errorf("TruncatedError = %+v", truncated)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want complete event plus partial event", len(events))
	}
	last := events[1]
	if !last.Truncated || last.Known || !reflect.DeepEqual(last.Words, []uint32{partial}) {
		t.Errorf("partial event = %+v", last)
	}
	if last.Timestamp != 20 {
		t.Errorf("partial event timestamp = %d, want 20", last.Timestamp)
	}
}

func TestSysEx7EOMAndBytes(t *testing.T) {
	t.Parallel()
	start := rawHeader(MTData64, 3, 0x1, 3) | 0xF001
	end := rawHeader(MTData64, 3, 0x3, 0x2) | 0x0304
	startData := uint32(0x02000000)
	endData := uint32(0x0506_0000)

	events, err := Parse([]uint32{start, startData, end, endData})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Kind != KindSysEx7 || !reflect.DeepEqual(events[0].Data, []byte{0xF0, 0x01, 0x02}) {
		t.Errorf("SysEx7 start = %+v", events[0])
	}
	if events[1].Kind != KindSysEx7 || !reflect.DeepEqual(events[1].Data, []byte{0x03, 0x04}) {
		t.Errorf("SysEx7 end = %+v", events[1])
	}
}

func TestParseBytesUsesRequestedOrder(t *testing.T) {
	t.Parallel()
	words := []uint32{
		midi1NoteOn(0, 1, 64, 65),
		midi2NoteOn(0, 1, 67, 0),
		0x11223344,
	}
	want := [][]uint32{words[:1], words[1:3]}

	for _, order := range []struct {
		name  string
		order binary.ByteOrder
	}{
		{"big endian", binary.BigEndian},
		{"little endian", binary.LittleEndian},
	} {
		order := order
		t.Run(order.name, func(t *testing.T) {
			t.Parallel()
			data := make([]byte, len(words)*4)
			for i, word := range words {
				order.order.PutUint32(data[i*4:], word)
			}
			events, err := ParseBytesOrder(data, order.order)
			if err != nil {
				t.Fatalf("ParseBytesOrder() error = %v", err)
			}
			if len(events) != len(want) {
				t.Fatalf("got %d events, want %d", len(events), len(want))
			}
			for i := range want {
				if !reflect.DeepEqual(events[i].Words, want[i]) {
					t.Errorf("event %d words = %#v, want %#v", i, events[i].Words, want[i])
				}
			}
		})
	}
}

func TestUtilityTimestamps(t *testing.T) {
	t.Parallel()
	clock := rawHeader(MTUtility, 0, 0x1, 0) | 0x1234
	timestamp := rawHeader(MTUtility, 0, 0x2, 0) | 0x5678

	events, err := Parse([]uint32{clock, timestamp})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if events[0].Kind != KindJRClock || events[0].Value != 0x1234 {
		t.Errorf("JR clock = %+v", events[0])
	}
	if events[1].Kind != KindJRTimestamp || events[1].Value != 0x5678 {
		t.Errorf("JR timestamp = %+v", events[1])
	}
}

func TestSysEx8DataAndUnknownDataStatus(t *testing.T) {
	t.Parallel()
	start := rawHeader(MTData128, 1, 0x1, 0x0D) | 0x07<<8 | 0x80
	events, err := Parse([]uint32{
		start,
		0x01020304,
		0x05060708,
		0x090A0B0C,
		rawHeader(MTData128, 1, 0x7, 0x01),
		0x11121314,
		0x15161718,
		0x191A1B1C,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	wantData := []byte{
		0x80, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
		0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
	}
	if events[0].Kind != KindSysEx8 || !reflect.DeepEqual(events[0].Data, wantData) {
		t.Errorf("SysEx8 = {kind:%d data:%#v}, want data %#v",
			events[0].Kind, events[0].Data, wantData)
	}
	if events[1].Known || events[1].Kind != KindUnknown {
		t.Errorf("unknown Data128 status = %+v", events[1])
	}
	if !reflect.DeepEqual(events[1].Words, []uint32{
		rawHeader(MTData128, 1, 0x7, 0x01), 0x11121314, 0x15161718, 0x191A1B1C,
	}) {
		t.Errorf("unknown Data128 words changed: %#v", events[1].Words)
	}
}

func TestProgramChangeBankFields(t *testing.T) {
	t.Parallel()
	w0 := rawHeader(MTMIDI2ChannelVoice, 2, 0xC, 3) | 1
	w1 := uint32(0x2A)<<24 | uint32(0x05)<<8 | 0x06
	events, err := Parse([]uint32{w0, w1})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got := events[0]
	if got.Kind != KindProgramChange || got.Channel != 3 ||
		!got.BankValid || got.Program != 0x2A ||
		got.BankMSB != 0x05 || got.BankLSB != 0x06 {
		t.Errorf("program change = %+v", got)
	}
}

func TestGroupEndIgnoresGrouplessStreamMessages(t *testing.T) {
	t.Parallel()
	words := []Word{
		{Value: midi1NoteOn(4, 0, 60, 1), Timestamp: 10},
		{Value: rawHeader(MTUMPStream, 0, 0x2, 0), Timestamp: 20},
		{Value: 0, Timestamp: 21},
		{Value: 0, Timestamp: 22},
		{Value: 0, Timestamp: 23},
		{Value: midi1NoteOn(4, 0, 61, 2), Timestamp: 30},
	}
	events, err := ParseTimedWithOptions(words, Options{EmitGroupEnd: true})
	if err != nil {
		t.Fatalf("ParseTimedWithOptions() error = %v", err)
	}
	var groupEnds int
	for _, ev := range events {
		if ev.Kind == KindGroupEnd {
			groupEnds++
		}
	}
	if groupEnds != 1 {
		t.Fatalf("got %d group-end events, want 1: %+v", groupEnds, events)
	}
	last := events[len(events)-1]
	if last.Kind != KindGroupEnd || last.Group != 4 || last.Timestamp != 30 {
		t.Errorf("final group end = %+v", last)
	}
}

func TestByteLengthValidation(t *testing.T) {
	t.Parallel()
	if _, err := ParseBytes([]byte{0x20, 0x90, 0x3C}); err == nil {
		t.Fatal("ParseBytes() accepted a partial 32-bit word")
	}
	events, err := Parse(nil)
	if err != nil || len(events) != 0 {
		t.Fatalf("Parse(nil) = (%v, %v), want empty result", events, err)
	}
}

func FuzzParseNeverPanics(f *testing.F) {
	f.Add([]byte{0x20, 0x90, 0x3C, 0x7F})
	f.Add([]byte{0x40, 0x90, 0x3C, 0x7F, 0xAB, 0xCD, 0x00, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		words := make([]uint32, len(data)/4)
		for i := range words {
			words[i] = binary.BigEndian.Uint32(data[i*4:])
		}
		_, _ = Parse(words)
	})
}

func rawHeader(mt MessageType, group, status, channel uint8) uint32 {
	return uint32(mt)<<28 | uint32(group&0x0F)<<24 |
		uint32(status&0x0F)<<20 | uint32(channel&0x0F)<<16
}

func midi1NoteOn(group, channel, note, velocity uint8) uint32 {
	return rawHeader(MTMIDI1ChannelVoice, group, 0x9, channel) |
		uint32(note)<<8 | uint32(velocity)
}

func midi2NoteOn(group, channel, note, attrType uint8) uint32 {
	return rawHeader(MTMIDI2ChannelVoice, group, 0x9, channel) |
		uint32(note)<<8 | uint32(attrType)
}

func perNoteController(group, channel, note, controller uint8) uint32 {
	return rawHeader(MTMIDI2ChannelVoice, group, 0x0, channel) |
		uint32(note)<<8 | uint32(controller)
}

func unknownMIDI2(group, status uint8) uint32 {
	return rawHeader(MTMIDI2ChannelVoice, group, status, 0)
}

package ump

// byteAt returns byte n of a UMP word, counted from the most significant byte.
func byteAt(word uint32, n uint) uint8 {
	return uint8(word >> (24 - n*8))
}

func low16(word uint32) uint16 {
	return uint16(word)
}

func decodeEvent(words []uint32, timestamp uint64) Event {
	ev := Event{
		Timestamp: timestamp,
		Words:     append([]uint32(nil), words...),
	}
	if len(words) == 0 {
		return ev
	}

	ev.Type = MessageType(words[0] >> 28)
	ev.Group = uint8(words[0] >> 24 & 0x0F)
	if ev.Type == MTUMPStream {
		ev.Group = 0
	}
	ev.Status = uint16(words[0] >> 20 & 0x0F)
	if len(words) < ev.Type.Words() {
		return ev
	}

	switch ev.Type {
	case MTUtility:
		decodeUtility(&ev)
	case MTSystem:
		decodeSystem(&ev)
	case MTMIDI1ChannelVoice:
		decodeMIDI1ChannelVoice(&ev)
	case MTMIDI2ChannelVoice:
		decodeMIDI2ChannelVoice(&ev)
	case MTData64:
		decodeData64(&ev)
	case MTData128:
		decodeData128(&ev)
	case MTFlexData:
		ev.Kind = KindFlexData
		ev.Known = true
	case MTUMPStream:
		decodeUMPStream(&ev)
	}
	return ev
}

func decodeUtility(ev *Event) {
	w := ev.Words[0]
	ev.Status = uint16(w >> 20 & 0x0F)
	ev.Value = w & 0xFFFFF
	switch ev.Status {
	case 0x0:
		ev.Known = true
	case 0x1:
		ev.Known = true
		ev.Kind = KindJRClock
		ev.Value = uint32(low16(w))
	case 0x2:
		ev.Known = true
		ev.Kind = KindJRTimestamp
		ev.Value = uint32(low16(w))
	case 0x3:
		ev.Known = true
		ev.Kind = KindDeltaClockstampTPQ
		ev.Value = uint32(low16(w))
	case 0x4:
		ev.Known = true
		ev.Kind = KindDeltaClockstamp
	}
}

func decodeSystem(ev *Event) {
	w := ev.Words[0]
	ev.Status = uint16(byteAt(w, 1))
	ev.Value = w & 0xFFFF
	switch ev.Status {
	case 0xF1, 0xF2, 0xF3, 0xF6, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFE, 0xFF:
		ev.Known = true
		ev.Kind = KindSystem
	}
}

func decodeMIDI1ChannelVoice(ev *Event) {
	w := ev.Words[0]
	ev.Status = uint16(byteAt(w, 1) >> 4)
	ev.Channel = byteAt(w, 1) & 0x0F
	data1 := byteAt(w, 2)
	data2 := byteAt(w, 3)

	switch ev.Status {
	case 0x8:
		ev.Known = true
		ev.Kind = KindNoteOff
		ev.Note = data1 & 0x7F
		ev.Velocity = uint32(data2 & 0x7F)
	case 0x9:
		ev.Known = true
		ev.Kind = KindNoteOn
		ev.Note = data1 & 0x7F
		ev.Velocity = uint32(data2 & 0x7F)
	case 0xA:
		ev.Known = true
		ev.Kind = KindPolyPressure
		ev.Note = data1 & 0x7F
		ev.Value = uint32(data2 & 0x7F)
	case 0xB:
		ev.Known = true
		ev.Kind = KindControlChange
		ev.Controller = data1 & 0x7F
		ev.Value = uint32(data2 & 0x7F)
	case 0xC:
		ev.Known = true
		ev.Kind = KindProgramChange
		ev.Program = data1 & 0x7F
	case 0xD:
		ev.Known = true
		ev.Kind = KindChannelPressure
		ev.Value = uint32(data1 & 0x7F)
	case 0xE:
		ev.Known = true
		ev.Kind = KindPitchBend
		ev.Value = uint32(data1&0x7F) | uint32(data2&0x7F)<<7
	}
}

func decodeMIDI2ChannelVoice(ev *Event) {
	w0, w1 := ev.Words[0], ev.Words[1]
	ev.Status = uint16(byteAt(w0, 1) >> 4)
	ev.Channel = byteAt(w0, 1) & 0x0F
	data1 := byteAt(w0, 2)
	data2 := byteAt(w0, 3)

	switch ev.Status {
	case 0x0:
		ev.Known = true
		ev.Kind = KindPerNoteRegisteredController
		ev.Note = data1
		ev.Controller = data2
		ev.Index = data2
		ev.Value = w1
	case 0x1:
		ev.Known = true
		ev.Kind = KindPerNoteAssignableController
		ev.Note = data1
		ev.Controller = data2
		ev.Index = data2
		ev.Value = w1
	case 0x2:
		ev.Known = true
		ev.Kind = KindRegisteredController
		ev.Bank = data1
		ev.Index = data2
		ev.Value = w1
	case 0x3:
		ev.Known = true
		ev.Kind = KindAssignableController
		ev.Bank = data1
		ev.Index = data2
		ev.Value = w1
	case 0x4:
		ev.Known = true
		ev.Kind = KindRelativeRegisteredController
		ev.Bank = data1
		ev.Index = data2
		ev.Value = w1
	case 0x5:
		ev.Known = true
		ev.Kind = KindRelativeAssignableController
		ev.Bank = data1
		ev.Index = data2
		ev.Value = w1
	case 0x6:
		ev.Known = true
		ev.Kind = KindPerNotePitchBend
		ev.Note = data1
		ev.Value = w1
	case 0x8:
		ev.Known = true
		ev.Kind = KindNoteOff
		ev.Note = data1
		ev.AttributeType = data2
		ev.Velocity = uint32(w1 >> 16)
		ev.AttributeData = uint16(w1)
	case 0x9:
		ev.Known = true
		ev.Kind = KindNoteOn
		ev.Note = data1
		ev.AttributeType = data2
		ev.Velocity = uint32(w1 >> 16)
		ev.AttributeData = uint16(w1)
	case 0xA:
		ev.Known = true
		ev.Kind = KindPolyPressure
		ev.Note = data1
		ev.Value = w1
	case 0xB:
		ev.Known = true
		ev.Kind = KindControlChange
		ev.Controller = data1
		ev.Value = w1
	case 0xC:
		ev.Known = true
		ev.Kind = KindProgramChange
		ev.BankValid = w0&1 != 0
		ev.Program = byteAt(w1, 0)
		ev.BankMSB = byteAt(w1, 2)
		ev.BankLSB = byteAt(w1, 3)
	case 0xD:
		ev.Known = true
		ev.Kind = KindChannelPressure
		ev.Value = w1
	case 0xE:
		ev.Known = true
		ev.Kind = KindPitchBend
		ev.Value = w1
	case 0xF:
		ev.Known = true
		ev.Kind = KindPerNoteManagement
		ev.Note = data1
		ev.AttributeData = uint16(data2)
	}
}

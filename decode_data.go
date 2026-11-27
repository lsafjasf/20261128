package ump

func appendMSBBytes(dst []byte, word uint32, count int) []byte {
	for i := 0; i < count; i++ {
		dst = append(dst, byteAt(word, uint(i)))
	}
	return dst
}

func appendWordBytes(dst []byte, word uint32, start, count int) []byte {
	for i := 0; i < count; i++ {
		dst = append(dst, byteAt(word, uint(start+i)))
	}
	return dst
}

func decodeData64(ev *Event) {
	w0, w1 := ev.Words[0], ev.Words[1]
	ev.Status = uint16(byteAt(w0, 1) >> 4)
	byteCount := int(byteAt(w0, 1) & 0x0F)
	if ev.Status > 0x3 {
		return
	}
	ev.Known = true
	ev.Kind = KindSysEx7

	limit := min(byteCount, 6)
	data := make([]byte, 0, limit)
	data = appendWordBytes(data, w0, 2, min(limit, 2))
	if limit > 2 {
		data = appendMSBBytes(data, w1, limit-2)
	}
	ev.Data = data
}

func decodeData128(ev *Event) {
	w0 := ev.Words[0]
	ev.Status = uint16(byteAt(w0, 1) >> 4)
	byteCount := int(byteAt(w0, 1) & 0x0F)

	switch ev.Status {
	case 0x0, 0x1, 0x2, 0x3:
		ev.Known = true
		ev.Kind = KindSysEx8
	case 0x8, 0x9:
		ev.Known = true
		ev.Kind = KindMixedDataSet
		return
	default:
		return
	}

	limit := min(byteCount, 13)
	data := make([]byte, 0, limit)
	if limit > 0 {
		data = appendWordBytes(data, w0, 3, 1)
	}
	for i := 1; i < len(ev.Words) && len(data) < limit; i++ {
		data = appendMSBBytes(data, ev.Words[i], min(4, limit-len(data)))
	}
	ev.Data = data
}

func decodeUMPStream(ev *Event) {
	w := ev.Words[0]
	ev.Status = uint16((w >> 16) & 0x3FF)
	ev.Value = w & 0xFFFF
	ev.Kind = KindUMPStream
	ev.Known = true
}

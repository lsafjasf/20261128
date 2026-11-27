package mp4

// buildStbl serializes stsd/stts/ctts/stss/stsc/stsz/stco for one track.
func buildStbl(tr *MuxTrack, chunkOffs []uint64) []byte {
	stbl := &writer{}
	stbl.bytes(boxBytes("stsd", stsdPayload(tr)))
	stbl.bytes(boxBytes("stts", sttsPayload(tr)))
	if c := cttsPayload(tr); c != nil {
		stbl.bytes(boxBytes("ctts", c))
	}
	if s := stssPayload(tr); s != nil {
		stbl.bytes(boxBytes("stss", s))
	}
	stbl.bytes(boxBytes("stsc", stscPayload()))
	stbl.bytes(boxBytes("stsz", stszPayload(tr)))
	stbl.bytes(boxBytes("stco", stcoPayload(chunkOffs)))
	return boxBytes("stbl", stbl.buf)
}

func stsdPayload(tr *MuxTrack) []byte {
	w := &writer{}
	w.u32(0) // version/flags
	w.u32(1) // entry count
	entry := &writer{}
	if tr.Handler == "vide" {
		entry.bytes(make([]byte, 6))
		entry.u16(1) // data reference index
		entry.bytes(make([]byte, 16))
		entry.u16(uint16(tr.Width))
		entry.u16(uint16(tr.Height))
		entry.u32(0x00480000) // horizresolution 72dpi
		entry.u32(0x00480000)
		entry.u32(0)
		entry.u16(1) // frame count
		entry.bytes(make([]byte, 32))
		entry.u16(0x0018) // depth
		entry.u16(0xffff)
		entry.bytes(boxBytes("avcC", []byte{1, 100, 0, 31, 0xff, 0xe0, 0}))
	} else {
		entry.bytes(make([]byte, 6))
		entry.u16(1)
		entry.bytes(make([]byte, 8))
		entry.u16(2)  // channelcount
		entry.u16(16) // samplesize
		entry.u16(0)  // pre_defined
		entry.u16(0)  // reserved
		entry.u32(48000 << 16)
		entry.bytes(boxBytes("esds", []byte{0, 0, 0, 0}))
	}
	w.u32(uint32(len(entry.buf) + 8))
	w.str(tr.Codec)
	w.bytes(entry.buf)
	return w.buf
}

func sttsPayload(tr *MuxTrack) []byte {
	type run struct{ count, delta uint32 }
	var runs []run
	// stts deltas are the steps between consecutive decode timestamps;
	// the final sample uses its own duration.
	for i, s := range tr.Samples {
		delta := s.Dur
		if i+1 < len(tr.Samples) {
			delta = tr.Samples[i+1].DTS - s.DTS
		}
		if n := len(runs); n > 0 && runs[n-1].delta == uint32(delta) {
			runs[n-1].count++
		} else {
			runs = append(runs, run{1, uint32(delta)})
		}
	}
	w := &writer{}
	w.u32(0)
	w.u32(uint32(len(runs)))
	for _, r := range runs {
		w.u32(r.count)
		w.u32(r.delta)
	}
	return w.buf
}

func cttsPayload(tr *MuxTrack) []byte {
	hasOffset := false
	neg := false
	for _, s := range tr.Samples {
		if s.CTS != s.DTS {
			hasOffset = true
			if s.CTS < s.DTS {
				neg = true
			}
		}
	}
	if !hasOffset {
		return nil
	}
	type run struct {
		count  uint32
		offset int32
	}
	var runs []run
	for _, s := range tr.Samples {
		off := int32(s.CTS - s.DTS)
		if n := len(runs); n > 0 && runs[n-1].offset == off {
			runs[n-1].count++
		} else {
			runs = append(runs, run{1, off})
		}
	}
	w := &writer{}
	if neg {
		w.u8(1) // version 1: signed offsets
	} else {
		w.u8(0)
	}
	w.u24(0)
	w.u32(uint32(len(runs)))
	for _, r := range runs {
		w.u32(r.count)
		w.i32(r.offset)
	}
	return w.buf
}

func stssPayload(tr *MuxTrack) []byte {
	if tr.Handler != "vide" {
		return nil
	}
	var syncs []uint32
	all := true
	for i, s := range tr.Samples {
		if s.Sync {
			syncs = append(syncs, uint32(i+1))
		} else {
			all = false
		}
	}
	if all {
		return nil
	}
	w := &writer{}
	w.u32(0)
	w.u32(uint32(len(syncs)))
	for _, s := range syncs {
		w.u32(s)
	}
	return w.buf
}

func stscPayload() []byte {
	w := &writer{}
	w.u32(0)
	w.u32(1)
	w.u32(1) // first chunk
	w.u32(1) // samples per chunk
	w.u32(1) // sample description index
	return w.buf
}

func stszPayload(tr *MuxTrack) []byte {
	w := &writer{}
	w.u32(0)
	w.u32(0) // no default size
	w.u32(uint32(len(tr.Samples)))
	for _, s := range tr.Samples {
		w.u32(uint32(len(s.Data)))
	}
	return w.buf
}

func stcoPayload(chunkOffs []uint64) []byte {
	w := &writer{}
	w.u32(0)
	w.u32(uint32(len(chunkOffs)))
	for _, o := range chunkOffs {
		w.u32(uint32(o))
	}
	return w.buf
}

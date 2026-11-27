package mp4

// buildTrak serializes one trak box (tkhd, optional edts, mdia with full stbl).
func buildTrak(tr *MuxTrack, chunkOffs []uint64, movieTimescale uint32) []byte {
	var mediaDur int64
	if n := len(tr.Samples); n > 0 {
		mediaDur = tr.Samples[n-1].DTS + tr.Samples[n-1].Dur
	}
	trackDur := mediaDur * int64(movieTimescale) / int64(tr.Timescale)

	tkhd := &writer{}
	tkhd.u32(0x00000007)
	tkhd.u32(0)
	tkhd.u32(0)
	tkhd.u32(tr.ID)
	tkhd.u32(0)
	tkhd.u32(uint32(trackDur))
	tkhd.u32(0)
	tkhd.u32(0)
	tkhd.u16(0)
	tkhd.u16(0)
	if tr.Handler == "soun" {
		tkhd.u16(0x0100)
	} else {
		tkhd.u16(0)
	}
	tkhd.u16(0)
	for _, v := range []uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000} {
		tkhd.u32(v)
	}
	if tr.Handler == "vide" {
		tkhd.u32(tr.Width << 16)
		tkhd.u32(tr.Height << 16)
	} else {
		tkhd.u32(0)
		tkhd.u32(0)
	}

	mdhd := &writer{}
	mdhd.u32(0)
	mdhd.u32(0)
	mdhd.u32(0)
	mdhd.u32(tr.Timescale)
	mdhd.u32(uint32(mediaDur))
	mdhd.u16(0x55c4)
	mdhd.u16(0)

	hdlr := &writer{}
	hdlr.u32(0)
	hdlr.u32(0)
	hdlr.str(tr.Handler)
	hdlr.u32(0)
	hdlr.u32(0)
	hdlr.u32(0)
	if tr.Handler == "vide" {
		hdlr.str("VideoHandler\x00")
	} else {
		hdlr.str("SoundHandler\x00")
	}

	minf := &writer{}
	if tr.Handler == "vide" {
		vmhd := &writer{}
		vmhd.u32(1)
		vmhd.u16(0)
		vmhd.u16(0)
		vmhd.u16(0)
		vmhd.u16(0)
		minf.bytes(boxBytes("vmhd", vmhd.buf))
	} else {
		smhd := &writer{}
		smhd.u32(0)
		smhd.u16(0)
		smhd.u16(0)
		minf.bytes(boxBytes("smhd", smhd.buf))
	}
	dref := &writer{}
	dref.u32(0)
	dref.u32(1)
	url := &writer{}
	url.u32(1)
	dref.bytes(boxBytes("url ", url.buf))
	dinf := &writer{}
	dinf.bytes(boxBytes("dref", dref.buf))
	minf.bytes(boxBytes("dinf", dinf.buf))
	minf.bytes(buildStbl(tr, chunkOffs))

	mdia := &writer{}
	mdia.bytes(boxBytes("mdhd", mdhd.buf))
	mdia.bytes(boxBytes("hdlr", hdlr.buf))
	mdia.bytes(boxBytes("minf", minf.buf))

	payload := &writer{}
	payload.bytes(boxBytes("tkhd", tkhd.buf))
	if len(tr.Elst) > 0 {
		elst := &writer{}
		elst.u32(0)
		elst.u32(uint32(len(tr.Elst)))
		for _, e := range tr.Elst {
			elst.u32(uint32(e.SegmentDuration))
			elst.i32(int32(e.MediaTime))
			elst.u16(uint16(e.RateInteger))
			elst.u16(0)
		}
		edts := &writer{}
		edts.bytes(boxBytes("elst", elst.buf))
		payload.bytes(boxBytes("edts", edts.buf))
	}
	payload.bytes(boxBytes("mdia", mdia.buf))
	return boxBytes("trak", payload.buf)
}

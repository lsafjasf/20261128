package mp4

import "fmt"

type stscEntry struct {
	FirstChunk      uint32
	SamplesPerChunk uint32
	DescIndex       uint32
}

func parseStts(p []byte) []SttsEntry {
	c := &cursor{buf: p}
	c.skip(4) // version/flags
	n := c.u32()
	out := make([]SttsEntry, 0, n)
	for i := uint32(0); i < n; i++ {
		out = append(out, SttsEntry{Count: c.u32(), Delta: c.u32()})
	}
	return out
}

func parseCtts(p []byte) []CttsEntry {
	c := &cursor{buf: p}
	version := c.u8()
	c.skip(3)
	n := c.u32()
	out := make([]CttsEntry, 0, n)
	for i := uint32(0); i < n; i++ {
		count := c.u32()
		var off int32
		if version == 1 {
			off = c.i32()
		} else {
			off = int32(c.u32())
		}
		out = append(out, CttsEntry{Count: count, Offset: off})
	}
	return out
}

func parseStss(p []byte) []uint32 {
	c := &cursor{buf: p}
	c.skip(4)
	n := c.u32()
	out := make([]uint32, 0, n)
	for i := uint32(0); i < n; i++ {
		out = append(out, c.u32())
	}
	return out
}

func parseStsc(p []byte) []stscEntry {
	c := &cursor{buf: p}
	c.skip(4)
	n := c.u32()
	out := make([]stscEntry, 0, n)
	for i := uint32(0); i < n; i++ {
		out = append(out, stscEntry{c.u32(), c.u32(), c.u32()})
	}
	return out
}

func parseStsz(p []byte) []uint32 {
	c := &cursor{buf: p}
	c.skip(4)
	def := c.u32()
	n := c.u32()
	out := make([]uint32, 0, n)
	if def != 0 {
		for i := uint32(0); i < n; i++ {
			out = append(out, def)
		}
		return out
	}
	for i := uint32(0); i < n; i++ {
		out = append(out, c.u32())
	}
	return out
}

func parseStco(p []byte, wide bool) []uint64 {
	c := &cursor{buf: p}
	c.skip(4)
	n := c.u32()
	out := make([]uint64, 0, n)
	for i := uint32(0); i < n; i++ {
		if wide {
			out = append(out, c.u64())
		} else {
			out = append(out, uint64(c.u32()))
		}
	}
	return out
}

func parseElst(p []byte) []ElstEntry {
	c := &cursor{buf: p}
	version := c.u8()
	c.skip(3)
	n := c.u32()
	out := make([]ElstEntry, 0, n)
	for i := uint32(0); i < n; i++ {
		var e ElstEntry
		if version == 1 {
			e.SegmentDuration = c.i64()
			e.MediaTime = c.i64()
		} else {
			e.SegmentDuration = int64(c.u32())
			e.MediaTime = int64(c.i32())
		}
		e.RateInteger = int16(c.u16())
		c.skip(2) // rate fraction
		out = append(out, e)
	}
	return out
}

// buildSamples expands stts/ctts/stss/stsc/stsz/stco into a per-sample list.
func (t *Track) buildSamples(stsc []stscEntry, stsz []uint32, stco []uint64) error {
	n := len(stsz)
	t.Samples = make([]Sample, n)

	// DTS and duration from stts.
	var dts int64
	idx := 0
	for _, e := range t.Stts {
		for k := uint32(0); k < e.Count && idx < n; k++ {
			t.Samples[idx].Index = idx
			t.Samples[idx].DTS = dts
			t.Samples[idx].Dur = int64(e.Delta)
			dts += int64(e.Delta)
			idx++
		}
	}
	if idx != n {
		return fmt.Errorf("trak %d: stts covers %d samples, stsz has %d", t.ID, idx, n)
	}

	// CTS = DTS + ctts offset.
	idx = 0
	for _, e := range t.Ctts {
		for k := uint32(0); k < e.Count && idx < n; k++ {
			t.Samples[idx].CTS = t.Samples[idx].DTS + int64(e.Offset)
			idx++
		}
	}
	for ; idx < n; idx++ {
		t.Samples[idx].CTS = t.Samples[idx].DTS
	}

	// Sync flags.
	if len(t.Stss) == 0 {
		for i := range t.Samples {
			t.Samples[i].Sync = true
		}
	} else {
		for _, s := range t.Stss {
			if s >= 1 && int(s) <= n {
				t.Samples[s-1].Sync = true
			}
		}
	}

	// Chunk layout -> sample offsets and sizes.
	sampleIdx := 0
	for ci, chunkOff := range stco {
		chunk := uint32(ci + 1)
		var spc uint32
		for _, e := range stsc {
			if e.FirstChunk <= chunk {
				spc = e.SamplesPerChunk
			} else {
				break
			}
		}
		off := chunkOff
		for k := uint32(0); k < spc && sampleIdx < n; k++ {
			t.Samples[sampleIdx].Off = int64(off)
			t.Samples[sampleIdx].Size = int64(stsz[sampleIdx])
			off += uint64(stsz[sampleIdx])
			sampleIdx++
		}
	}
	if sampleIdx != n {
		return fmt.Errorf("trak %d: chunk tables cover %d of %d samples", t.ID, sampleIdx, n)
	}
	return nil
}

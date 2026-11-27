package mp4

import (
	"fmt"
	"os"
)

// MuxSample is one sample to be written, in decode order within its track.
type MuxSample struct {
	DTS  int64
	CTS  int64
	Dur  int64
	Sync bool
	Data []byte
}

// MuxTrack describes one output track.
type MuxTrack struct {
	ID        uint32
	Handler   string // "vide" or "soun"
	Codec     string // e.g. "avc1", "mp4a"
	Timescale uint32
	Width     uint32 // video only, pixels
	Height    uint32
	Elst      []ElstEntry // optional; nil = no edts
	Samples   []MuxSample
}

// Mux writes a non-fragmented MP4 with ftyp, interleaved mdat, then moov.
// Samples are interleaved by DTS normalized to the movie timescale.
func Mux(path string, tracks []*MuxTrack, movieTimescale uint32) error {
	if len(tracks) == 0 {
		return fmt.Errorf("no tracks")
	}
	ftyp := boxBytes("ftyp", ftypPayload())

	// Interleave samples across tracks by scaled DTS.
	type ref struct{ trak, idx int }
	var order []ref
	pos := make([]int, len(tracks))
	for {
		best := -1
		var bestKey numDen
		for ti, tr := range tracks {
			if pos[ti] >= len(tr.Samples) {
				continue
			}
			key := numDen{tr.Samples[pos[ti]].DTS, int64(tr.Timescale)}
			if best == -1 || key.less(bestKey) {
				best = ti
				bestKey = key
			}
		}
		if best == -1 {
			break
		}
		order = append(order, ref{best, pos[best]})
		pos[best]++
	}

	// Lay out mdat: one sample per chunk.
	mdatPayload := &writer{}
	chunkOffs := make([][]uint64, len(tracks))
	off := int64(len(ftyp) + 8) // ftyp + mdat header
	for _, r := range order {
		tr := tracks[r.trak]
		s := tr.Samples[r.idx]
		chunkOffs[r.trak] = append(chunkOffs[r.trak], uint64(off))
		mdatPayload.bytes(s.Data)
		off += int64(len(s.Data))
	}

	moov := buildMoov(tracks, chunkOffs, movieTimescale)

	out := &writer{}
	out.bytes(ftyp)
	out.bytes(boxBytes("mdat", mdatPayload.buf))
	out.bytes(moov)
	return os.WriteFile(path, out.buf, 0o644)
}

type numDen struct{ n, d int64 }

func (a numDen) less(b numDen) bool { return a.n*b.d < b.n*a.d }

func ftypPayload() []byte {
	w := &writer{}
	w.str("isom")
	w.u32(0x200)
	w.str("isom")
	w.str("iso2")
	w.str("avc1")
	w.str("mp41")
	return w.buf
}

func buildMoov(tracks []*MuxTrack, chunkOffs [][]uint64, movieTimescale uint32) []byte {
	var movieDur int64
	for _, tr := range tracks {
		var d int64
		if n := len(tr.Samples); n > 0 {
			d = tr.Samples[n-1].DTS + tr.Samples[n-1].Dur
		}
		md := d * int64(movieTimescale) / int64(tr.Timescale)
		if md > movieDur {
			movieDur = md
		}
	}
	mvhd := &writer{}
	mvhd.u32(0) // version/flags
	mvhd.u32(0)
	mvhd.u32(0) // creation/modification time
	mvhd.u32(movieTimescale)
	mvhd.u32(uint32(movieDur))
	mvhd.u32(0x00010000) // rate
	mvhd.u16(0x0100)     // volume
	mvhd.u16(0)
	mvhd.u32(0)
	mvhd.u32(0)
	for _, v := range []uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000} {
		mvhd.u32(v)
	}
	for i := 0; i < 6; i++ {
		mvhd.u32(0)
	}
	mvhd.u32(uint32(len(tracks) + 1)) // next track id

	payload := &writer{}
	payload.bytes(boxBytes("mvhd", mvhd.buf))
	for i, tr := range tracks {
		payload.bytes(buildTrak(tr, chunkOffs[i], movieTimescale))
	}
	return boxBytes("moov", payload.buf)
}

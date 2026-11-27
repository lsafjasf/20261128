package mp4

import (
	"fmt"
	"os"
)

// Sample is one media sample with fully resolved timing.
type Sample struct {
	Index int   // decode order index within the track
	DTS   int64 // decode timestamp in track timescale
	CTS   int64 // composition (presentation) timestamp in track timescale
	Dur   int64 // sample duration in track timescale
	Size  int64
	Off   int64 // absolute offset into the source file
	Sync  bool  // random access point (stss)
}

// SttsEntry is a time-to-sample run.
type SttsEntry struct {
	Count uint32
	Delta uint32
}

// CttsEntry is a composition-offset run (signed, ctts v1 semantics).
type CttsEntry struct {
	Count  uint32
	Offset int32
}

// ElstEntry is one edit list entry.
type ElstEntry struct {
	SegmentDuration int64 // in movie timescale; -1 = empty edit dwell
	MediaTime       int64 // in media timescale; -1 = empty edit
	RateInteger     int16
}

// Track holds the parsed tables of one trak.
type Track struct {
	ID        uint32
	Handler   string // "vide", "soun", ...
	Timescale uint32
	Duration  int64  // mdhd duration in track timescale
	Codec     string // stsd sample entry format
	Stts      []SttsEntry
	Ctts      []CttsEntry
	Stss      []uint32
	Elst      []ElstEntry
	Samples   []Sample
}

// File is a parsed MP4 with resolved sample tables.
type File struct {
	MajorBrand     string
	MovieTimescale uint32
	Tracks         []*Track
	raw            []byte
}

// Data returns the encoded payload of sample s of track t.
func (f *File) Data(t *Track, s Sample) []byte {
	return f.raw[s.Off : s.Off+s.Size]
}

// Video returns the first video track, or nil.
func (f *File) Video() *Track {
	for _, t := range f.Tracks {
		if t.Handler == "vide" {
			return t
		}
	}
	return nil
}

// Audio returns the first audio track, or nil.
func (f *File) Audio() *Track {
	for _, t := range f.Tracks {
		if t.Handler == "soun" {
			return t
		}
	}
	return nil
}

// Demux parses an MP4 file (non-fragmented) and resolves all sample tables.
func Demux(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	boxes, err := ReadBoxes(bytesReader(raw))
	if err != nil {
		return nil, err
	}
	f := &File{raw: raw}
	var moov *Box
	for _, b := range boxes {
		switch b.Type {
		case "ftyp":
			if len(b.Payload) >= 4 {
				f.MajorBrand = string(b.Payload[:4])
			}
		case "moov":
			moov = b
		}
	}
	if moov == nil {
		return nil, fmt.Errorf("no moov box found")
	}
	if mvhd := moov.Find("mvhd"); mvhd != nil {
		c := &cursor{buf: mvhd.Payload}
		version := c.u8()
		c.skip(3)
		if version == 1 {
			c.skip(16)
		} else {
			c.skip(8)
		}
		f.MovieTimescale = c.u32()
	}
	for _, trak := range moov.Children {
		if trak.Type != "trak" {
			continue
		}
		t, err := parseTrak(trak)
		if err != nil {
			return nil, err
		}
		f.Tracks = append(f.Tracks, t)
	}
	return f, nil
}

func parseTrak(trak *Box) (*Track, error) {
	t := &Track{}
	if tkhd := trak.Find("tkhd"); tkhd != nil {
		c := &cursor{buf: tkhd.Payload}
		version := c.u8()
		c.skip(3)
		if version == 1 {
			c.skip(16)
		} else {
			c.skip(8)
		}
		t.ID = c.u32()
	}
	mdia := trak.Find("mdia")
	if mdia == nil {
		return nil, fmt.Errorf("trak %d: no mdia", t.ID)
	}
	if mdhd := mdia.Find("mdhd"); mdhd != nil {
		c := &cursor{buf: mdhd.Payload}
		version := c.u8()
		c.skip(3)
		if version == 1 {
			c.skip(16)
			t.Timescale = c.u32()
			t.Duration = c.i64()
		} else {
			c.skip(8)
			t.Timescale = c.u32()
			t.Duration = int64(c.u32())
		}
	}
	if hdlr := mdia.Find("hdlr"); hdlr != nil {
		c := &cursor{buf: hdlr.Payload}
		c.skip(8) // version/flags + pre_defined
		t.Handler = c.str(4)
	}
	if elst := trak.Path("edts", "elst"); elst != nil {
		t.Elst = parseElst(elst.Payload)
	}
	stbl := mdia.Path("minf", "stbl")
	if stbl == nil {
		return nil, fmt.Errorf("trak %d: no stbl", t.ID)
	}
	if stsd := stbl.Find("stsd"); stsd != nil && len(stsd.Payload) >= 16 {
		t.Codec = string(stsd.Payload[12:16])
	}
	if stts := stbl.Find("stts"); stts != nil {
		t.Stts = parseStts(stts.Payload)
	}
	if ctts := stbl.Find("ctts"); ctts != nil {
		t.Ctts = parseCtts(ctts.Payload)
	}
	if stss := stbl.Find("stss"); stss != nil {
		t.Stss = parseStss(stss.Payload)
	}
	var stsc []stscEntry
	var stsz []uint32
	var stco []uint64
	if b := stbl.Find("stsc"); b != nil {
		stsc = parseStsc(b.Payload)
	}
	if b := stbl.Find("stsz"); b != nil {
		stsz = parseStsz(b.Payload)
	}
	if b := stbl.Find("stco"); b != nil {
		stco = parseStco(b.Payload, false)
	} else if b := stbl.Find("co64"); b != nil {
		stco = parseStco(b.Payload, true)
	}
	return t, t.buildSamples(stsc, stsz, stco)
}

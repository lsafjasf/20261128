package mp4safe

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Sample is one sample's byte extent in the source file.
type Sample struct {
	Index  uint32
	Offset uint64
	Size   uint32
}

// ChunkTable is a parsed stco or co64 box.
type ChunkTable struct {
	Box        *Box
	Wide       bool
	EntryStart uint64
	Entries    []uint64
}

// Track contains the sample-table information needed for hashing and rewriting.
type Track struct {
	ID          uint32
	Handler     string
	Chunks      *ChunkTable
	SampleSizes []uint32
	Samples     []Sample
	SampleCount uint32
	TotalBytes  uint64
}

type stscEntry struct {
	FirstChunk      uint32
	SamplesPerChunk uint32
}

func (f *File) collectTracks() error {
	moov := f.topLevel([4]byte{'m', 'o', 'o', 'v'})
	if moov == nil {
		return fmt.Errorf("no top-level moov box")
	}
	for _, trak := range moov.Children {
		if trak.Type != [4]byte{'t', 'r', 'a', 'k'} {
			continue
		}
		track, err := f.parseTrack(trak)
		if err != nil {
			return fmt.Errorf("track at %d: %w", trak.Start, err)
		}
		if track != nil {
			if f.trackByID(track.ID) != nil {
				return fmt.Errorf("duplicate track ID %d", track.ID)
			}
			f.Tracks = append(f.Tracks, track)
		}
	}
	return nil
}

func (f *File) trackByID(id uint32) *Track {
	for _, t := range f.Tracks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (f *File) parseTrack(trak *Box) (*Track, error) {
	t := &Track{}
	if tkhd := findChild(trak, [4]byte{'t', 'k', 'h', 'd'}); tkhd != nil {
		id, err := f.parseTrackID(tkhd)
		if err != nil {
			return nil, err
		}
		t.ID = id
	}
	if t.ID == 0 {
		return nil, fmt.Errorf("missing or zero track ID")
	}
	mdia := findChild(trak, [4]byte{'m', 'd', 'i', 'a'})
	if mdia == nil {
		return nil, fmt.Errorf("missing mdia")
	}
	if hdlr := findChild(mdia, [4]byte{'h', 'd', 'l', 'r'}); hdlr != nil {
		handler, err := f.parseHandler(hdlr)
		if err != nil {
			return nil, err
		}
		t.Handler = handler
	}
	minf := findChild(mdia, [4]byte{'m', 'i', 'n', 'f'})
	if minf == nil {
		return nil, fmt.Errorf("missing minf")
	}
	stbl := findChild(minf, [4]byte{'s', 't', 'b', 'l'})
	if stbl == nil {
		return nil, fmt.Errorf("missing stbl")
	}

	var err error
	if t.Chunks, err = f.parseChunkTable(stbl); err != nil {
		return nil, err
	}
	if t.SampleSizes, t.SampleCount, err = f.parseSampleSizes(stbl); err != nil {
		return nil, err
	}
	entries, err := f.parseSTSC(stbl)
	if err != nil {
		return nil, err
	}
	if t.SampleCount == 0 {
		if t.Chunks != nil && len(t.Chunks.Entries) != 0 {
			return nil, fmt.Errorf("chunk table has chunks but no samples")
		}
		return t, nil
	}
	if t.Chunks == nil {
		return nil, fmt.Errorf("sample sizes present without stco/co64")
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("missing stsc")
	}
	if err := t.buildSamples(entries); err != nil {
		return nil, err
	}
	return t, nil
}

func (f *File) parseTrackID(b *Box) (uint32, error) {
	p, err := f.payload(b)
	if err != nil {
		return 0, err
	}
	if len(p) < 4 {
		return 0, fmt.Errorf("tkhd too short")
	}
	switch p[0] {
	case 0:
		if len(p) < 16 {
			return 0, fmt.Errorf("tkhd v0 too short")
		}
		return binary.BigEndian.Uint32(p[12:16]), nil
	case 1:
		if len(p) < 24 {
			return 0, fmt.Errorf("tkhd v1 too short")
		}
		return binary.BigEndian.Uint32(p[20:24]), nil
	default:
		return 0, fmt.Errorf("unsupported tkhd version %d", p[0])
	}
}

func (f *File) parseHandler(b *Box) (string, error) {
	p, err := f.payload(b)
	if err != nil {
		return "", err
	}
	if len(p) < 12 {
		return "", fmt.Errorf("hdlr too short")
	}
	var typ [4]byte
	copy(typ[:], p[8:12])
	return TypeName(typ), nil
}

func (f *File) parseChunkTable(stbl *Box) (*ChunkTable, error) {
	var b *Box
	wide := false
	if b = findChild(stbl, [4]byte{'s', 't', 'c', 'o'}); b != nil {
		wide = false
	} else if b = findChild(stbl, [4]byte{'c', 'o', '6', '4'}); b != nil {
		wide = true
	} else {
		return nil, nil
	}
	p, err := f.payload(b)
	if err != nil {
		return nil, err
	}
	if len(p) < 8 {
		return nil, fmt.Errorf("%s too short", b.TypeName())
	}
	count := binary.BigEndian.Uint32(p[4:8])
	width := uint64(4)
	if wide {
		width = 8
	}
	if need := uint64(8) + uint64(count)*width; need > uint64(len(p)) {
		return nil, fmt.Errorf("%s entry count %d exceeds payload", b.TypeName(), count)
	}
	entries := make([]uint64, count)
	for i := uint32(0); i < count; i++ {
		off := 8 + uint64(i)*width
		if wide {
			entries[i] = binary.BigEndian.Uint64(p[off : off+8])
		} else {
			entries[i] = uint64(binary.BigEndian.Uint32(p[off : off+4]))
		}
	}
	return &ChunkTable{
		Box:        b,
		Wide:       wide,
		EntryStart: b.PayloadStart() + 8,
		Entries:    entries,
	}, nil
}

func (f *File) parseSampleSizes(stbl *Box) ([]uint32, uint32, error) {
	if b := findChild(stbl, [4]byte{'s', 't', 's', 'z'}); b != nil {
		p, err := f.payload(b)
		if err != nil {
			return nil, 0, err
		}
		if len(p) < 12 {
			return nil, 0, fmt.Errorf("stsz too short")
		}
		fixed := binary.BigEndian.Uint32(p[4:8])
		count := binary.BigEndian.Uint32(p[8:12])
		sizes := make([]uint32, count)
		if fixed != 0 {
			for i := range sizes {
				sizes[i] = fixed
			}
		} else {
			if need := uint64(12) + uint64(count)*4; need > uint64(len(p)) {
				return nil, 0, fmt.Errorf("stsz entry count %d exceeds payload", count)
			}
			for i := range sizes {
				off := 12 + i*4
				sizes[i] = binary.BigEndian.Uint32(p[off : off+4])
			}
		}
		return sizes, count, nil
	}
	if b := findChild(stbl, [4]byte{'s', 't', 'z', '2'}); b != nil {
		p, err := f.payload(b)
		if err != nil {
			return nil, 0, err
		}
		if len(p) < 12 {
			return nil, 0, fmt.Errorf("stz2 too short")
		}
		count := binary.BigEndian.Uint32(p[8:12])
		fieldSize := p[7]
		sizes, err := parseCompactSizes(p[12:], count, fieldSize)
		if err != nil {
			return nil, 0, err
		}
		return sizes, count, nil
	}
	return nil, 0, nil
}

func parseCompactSizes(data []byte, count uint32, fieldSize byte) ([]uint32, error) {
	sizes := make([]uint32, count)
	switch fieldSize {
	case 4:
		if uint64(len(data))*2 < uint64(count) {
			return nil, fmt.Errorf("stz2 4-bit size table truncated")
		}
		for i := uint32(0); i < count; i++ {
			v := data[i/2]
			if i%2 == 0 {
				sizes[i] = uint32(v >> 4)
			} else {
				sizes[i] = uint32(v & 0x0f)
			}
		}
	case 8:
		if uint64(len(data)) < uint64(count) {
			return nil, fmt.Errorf("stz2 8-bit size table truncated")
		}
		for i := uint32(0); i < count; i++ {
			sizes[i] = uint32(data[i])
		}
	case 16:
		if uint64(len(data)) < uint64(count)*2 {
			return nil, fmt.Errorf("stz2 16-bit size table truncated")
		}
		for i := uint32(0); i < count; i++ {
			sizes[i] = uint32(binary.BigEndian.Uint16(data[i*2 : i*2+2]))
		}
	default:
		return nil, fmt.Errorf("stz2 invalid field size %d", fieldSize)
	}
	return sizes, nil
}

func (f *File) parseSTSC(stbl *Box) ([]stscEntry, error) {
	b := findChild(stbl, [4]byte{'s', 't', 's', 'c'})
	if b == nil {
		return nil, nil
	}
	p, err := f.payload(b)
	if err != nil {
		return nil, err
	}
	if len(p) < 8 {
		return nil, fmt.Errorf("stsc too short")
	}
	count := binary.BigEndian.Uint32(p[4:8])
	if need := uint64(8) + uint64(count)*12; need > uint64(len(p)) {
		return nil, fmt.Errorf("stsc entry count %d exceeds payload", count)
	}
	entries := make([]stscEntry, count)
	for i := range entries {
		off := 8 + i*12
		entries[i] = stscEntry{
			FirstChunk:      binary.BigEndian.Uint32(p[off : off+4]),
			SamplesPerChunk: binary.BigEndian.Uint32(p[off+4 : off+8]),
		}
		if i == 0 && entries[i].FirstChunk != 1 {
			return nil, fmt.Errorf("stsc first_chunk is %d, want 1", entries[i].FirstChunk)
		}
		if i > 0 && entries[i].FirstChunk <= entries[i-1].FirstChunk {
			return nil, fmt.Errorf("stsc first_chunk values are not increasing")
		}
		if entries[i].SamplesPerChunk == 0 {
			return nil, fmt.Errorf("stsc samples_per_chunk is zero")
		}
	}
	return entries, nil
}

func (t *Track) buildSamples(entries []stscEntry) error {
	chunkCount := len(t.Chunks.Entries)
	if len(t.SampleSizes) == 0 {
		if chunkCount != 0 {
			return fmt.Errorf("chunk table has %d chunks but no samples", chunkCount)
		}
		return nil
	}
	if chunkCount == 0 {
		return fmt.Errorf("samples present without chunks")
	}
	sampleIndex := 0
	for chunk := 0; chunk < chunkCount; chunk++ {
		spc, err := samplesForChunk(entries, chunk+1)
		if err != nil {
			return err
		}
		next := t.Chunks.Entries[chunk]
		for i := uint32(0); i < spc; i++ {
			if sampleIndex >= len(t.SampleSizes) {
				return fmt.Errorf("chunk %d contains more samples than stsz count %d", chunk+1, len(t.SampleSizes))
			}
			size := t.SampleSizes[sampleIndex]
			if next < t.Chunks.Entries[chunk] {
				return fmt.Errorf("sample offsets overflow in chunk %d", chunk+1)
			}
			s := Sample{Index: uint32(sampleIndex), Offset: next, Size: size}
			t.Samples = append(t.Samples, s)
			if math.MaxUint64-t.TotalBytes < uint64(size) {
				return fmt.Errorf("track sample byte count overflow")
			}
			t.TotalBytes += uint64(size)
			if math.MaxUint64-next < uint64(size) {
				return fmt.Errorf("sample offset overflow in chunk %d", chunk+1)
			}
			next += uint64(size)
			sampleIndex++
		}
	}
	if sampleIndex != len(t.SampleSizes) {
		return fmt.Errorf("sample tables describe %d samples, stsz declares %d", sampleIndex, len(t.SampleSizes))
	}
	return nil
}

func samplesForChunk(entries []stscEntry, chunk int) (uint32, error) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := (lo + hi) / 2
		if int(entries[mid].FirstChunk) <= chunk {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == 0 {
		return 0, fmt.Errorf("no stsc entry covers chunk %d", chunk)
	}
	return entries[lo-1].SamplesPerChunk, nil
}

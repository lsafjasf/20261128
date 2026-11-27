// Package fontmetrics reads the horizontal metrics needed for subtitle layout
// directly from TrueType/OpenType files. It intentionally does not rasterize
// glyphs.
package fontmetrics

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

type tableRef struct {
	offset uint32
	length uint32
}

// Face implements layout.MetricFace using a single sfnt face.
type Face struct {
	data            []byte
	unitsPerEm      float64
	ascent          float64
	descent         float64
	numberOfHMetric uint16
	numGlyphs       uint16
	advances        []uint16
	cmap            map[uint32]uint16
}

// Load reads the first face in a TTF, OTF, or TTC file.
func Load(path string) (*Face, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	face, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return face, nil
}

// Parse reads a single sfnt face from data. For a TTC, the first face is used.
func Parse(data []byte) (*Face, error) {
	base, err := firstFaceOffset(data)
	if err != nil {
		return nil, err
	}
	tables, err := readTables(data, base)
	if err != nil {
		return nil, err
	}
	head, ok := tables["head"]
	if !ok {
		return nil, errors.New("missing head table")
	}
	hhea, ok := tables["hhea"]
	if !ok {
		return nil, errors.New("missing hhea table")
	}
	maxp, ok := tables["maxp"]
	if !ok {
		return nil, errors.New("missing maxp table")
	}
	hmtx, ok := tables["hmtx"]
	if !ok {
		return nil, errors.New("missing hmtx table")
	}
	cmapTable, ok := tables["cmap"]
	if !ok {
		return nil, errors.New("missing cmap table")
	}

	unitValue, err := readU16(data, head.offset, 18)
	if err != nil || unitValue == 0 {
		return nil, errors.New("invalid unitsPerEm")
	}
	ascender, err := readI16(data, hhea.offset, 4)
	if err != nil {
		return nil, fmt.Errorf("read ascender: %w", err)
	}
	descender, err := readI16(data, hhea.offset, 6)
	if err != nil {
		return nil, fmt.Errorf("read descender: %w", err)
	}
	numberOfHMetric, err := readU16(data, hhea.offset, 34)
	if err != nil {
		return nil, fmt.Errorf("read hhea metric count: %w", err)
	}
	numGlyphs, err := readU16(data, maxp.offset, 4)
	if err != nil {
		return nil, fmt.Errorf("read glyph count: %w", err)
	}
	if numberOfHMetric > numGlyphs {
		numberOfHMetric = numGlyphs
	}
	advances, err := readAdvances(data, hmtx, numberOfHMetric, numGlyphs)
	if err != nil {
		return nil, err
	}
	cmap, err := parseCmap(data, cmapTable)
	if err != nil {
		return nil, err
	}

	face := &Face{
		data:            data,
		unitsPerEm:      float64(unitValue),
		ascent:          float64(ascender) / float64(unitValue),
		descent:         float64(-descender) / float64(unitValue),
		numberOfHMetric: numberOfHMetric,
		numGlyphs:       numGlyphs,
		advances:        advances,
		cmap:            cmap,
	}
	if face.ascent <= 0 || face.descent <= 0 {
		face.ascent, face.descent = 0.88, 0.12
	}
	return face, nil
}

// Advance returns the glyph advance in em units, or false when the face has no
// mapping for rune.
func (f *Face) Advance(r rune) (float64, bool) {
	glyph, ok := f.cmap[uint32(r)]
	if !ok || glyph == 0 || len(f.advances) == 0 {
		return 0, false
	}
	index := int(glyph)
	if index >= len(f.advances) {
		index = len(f.advances) - 1
	}
	return float64(f.advances[index]) / f.unitsPerEm, true
}

func (f *Face) Ascent() float64  { return f.ascent }
func (f *Face) Descent() float64 { return f.descent }

func firstFaceOffset(data []byte) (uint32, error) {
	if len(data) < 12 {
		return 0, errors.New("font data is truncated")
	}
	if string(data[:4]) != "ttcf" {
		return 0, nil
	}
	count := binary.BigEndian.Uint32(data[8:12])
	if count == 0 || len(data) < 16 {
		return 0, errors.New("TTC contains no faces")
	}
	return binary.BigEndian.Uint32(data[12:16]), nil
}

func readTables(data []byte, base uint32) (map[string]tableRef, error) {
	if int(base)+12 > len(data) {
		return nil, errors.New("sfnt header is truncated")
	}
	version := string(data[base : base+4])
	if version != "\x00\x01\x00\x00" && version != "OTTO" && version != "true" && version != "typ1" {
		return nil, fmt.Errorf("unsupported sfnt version %q", version)
	}
	count := int(binary.BigEndian.Uint16(data[base+4 : base+6]))
	start := int(base) + 12
	if count < 0 || start+count*16 > len(data) {
		return nil, errors.New("sfnt table directory is truncated")
	}
	tables := make(map[string]tableRef, count)
	for i := 0; i < count; i++ {
		record := data[start+i*16 : start+(i+1)*16]
		tag := string(record[:4])
		offset := binary.BigEndian.Uint32(record[8:12])
		length := binary.BigEndian.Uint32(record[12:16])
		if uint64(offset)+uint64(length) > uint64(len(data)) {
			return nil, fmt.Errorf("table %q is outside file bounds", tag)
		}
		tables[tag] = tableRef{offset: offset, length: length}
	}
	return tables, nil
}

func readAdvances(data []byte, hmtx tableRef, metricCount, glyphCount uint16) ([]uint16, error) {
	if metricCount == 0 || glyphCount == 0 {
		return nil, errors.New("font has no horizontal metrics")
	}
	advances := make([]uint16, int(glyphCount))
	last := uint16(0)
	for i := 0; i < int(metricCount); i++ {
		value, err := readU16(data, hmtx.offset, uint32(i*4))
		if err != nil {
			return nil, fmt.Errorf("read hmtx advance %d: %w", i, err)
		}
		advances[i] = value
		last = value
	}
	for i := int(metricCount); i < int(glyphCount); i++ {
		advances[i] = last
	}
	return advances, nil
}

func readU16(data []byte, base, offset uint32) (uint16, error) {
	position := uint64(base) + uint64(offset)
	if position+2 > uint64(len(data)) {
		return 0, errors.New("read past end of font")
	}
	return binary.BigEndian.Uint16(data[position : position+2]), nil
}

func readI16(data []byte, base, offset uint32) (int16, error) {
	value, err := readU16(data, base, offset)
	return int16(value), err
}

func readU32(data []byte, base, offset uint32) (uint32, error) {
	position := uint64(base) + uint64(offset)
	if position+4 > uint64(len(data)) {
		return 0, errors.New("read past end of font")
	}
	return binary.BigEndian.Uint32(data[position : position+4]), nil
}

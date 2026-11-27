package fontmetrics

import (
	"errors"
	"fmt"
	"sort"
)

type cmapCandidate struct {
	offset uint32
	format uint16
	score  int
}

func parseCmap(data []byte, table tableRef) (map[uint32]uint16, error) {
	count, err := readU16(data, table.offset, 2)
	if err != nil {
		return nil, fmt.Errorf("read cmap header: %w", err)
	}
	candidates := make([]cmapCandidate, 0, count)
	for i := 0; i < int(count); i++ {
		record := uint32(4 + i*8)
		platform, err := readU16(data, table.offset, record)
		if err != nil {
			return nil, fmt.Errorf("read cmap record: %w", err)
		}
		encoding, err := readU16(data, table.offset, record+2)
		if err != nil {
			return nil, fmt.Errorf("read cmap encoding: %w", err)
		}
		subtable, err := readU32(data, table.offset, record+4)
		if err != nil {
			return nil, fmt.Errorf("read cmap offset: %w", err)
		}
		format, err := readU16(data, table.offset, subtable)
		if err != nil {
			continue
		}
		score := cmapScore(platform, encoding, format)
		if score > 0 {
			candidates = append(candidates, cmapCandidate{
				offset: subtable,
				format: format,
				score:  score,
			})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	glyphs := make(map[uint32]uint16)
	for _, candidate := range candidates {
		var err error
		switch candidate.format {
		case 0:
			err = parseCmap0(data, table, candidate.offset, glyphs)
		case 4:
			err = parseCmap4(data, table, candidate.offset, glyphs)
		case 6:
			err = parseCmap6(data, table, candidate.offset, glyphs)
		case 12, 13:
			err = parseCmap12(data, table, candidate.offset, candidate.format == 13, glyphs)
		}
		if err != nil {
			continue
		}
	}
	if len(glyphs) == 0 {
		return nil, errors.New("no supported Unicode cmap subtable")
	}
	return glyphs, nil
}

func cmapScore(platform, encoding, format uint16) int {
	score := 0
	switch {
	case platform == 3 && encoding == 10:
		score = 100
	case platform == 0:
		score = 90
	case platform == 3 && encoding == 1:
		score = 80
	case platform == 3 && encoding == 0:
		score = 60
	default:
		return 0
	}
	switch format {
	case 12, 13:
		score += 20
	case 4, 6:
		score += 10
	case 0:
		score += 1
	}
	return score
}

func parseCmap0(data []byte, table tableRef, offset uint32, glyphs map[uint32]uint16) error {
	length, err := readU16(data, table.offset, offset+2)
	if err != nil {
		return err
	}
	if length < 262 {
		return errors.New("cmap format 0 is truncated")
	}
	for code := uint32(0); code < 256; code++ {
		glyph, err := readU16(data, table.offset, offset+6+code*2)
		if err != nil {
			return err
		}
		addGlyph(glyphs, code, glyph)
	}
	return nil
}

func parseCmap4(data []byte, table tableRef, offset uint32, glyphs map[uint32]uint16) error {
	segCountX2, err := readU16(data, table.offset, offset+6)
	if err != nil {
		return err
	}
	segCount := int(segCountX2 / 2)
	if segCount == 0 {
		return errors.New("cmap format 4 has no segments")
	}
	endCodes := offset + 14
	startCodes := endCodes + uint32(segCount)*2 + 2
	idDeltas := startCodes + uint32(segCount)*2
	idRangeOffsets := idDeltas + uint32(segCount)*2
	for i := 0; i < segCount; i++ {
		end, err := readU16(data, table.offset, endCodes+uint32(i)*2)
		if err != nil {
			return err
		}
		start, err := readU16(data, table.offset, startCodes+uint32(i)*2)
		if err != nil {
			return err
		}
		delta, err := readU16(data, table.offset, idDeltas+uint32(i)*2)
		if err != nil {
			return err
		}
		rangeOffset, err := readU16(data, table.offset, idRangeOffsets+uint32(i)*2)
		if err != nil {
			return err
		}
		if start > end {
			continue
		}
		for code := uint32(start); code <= uint32(end); code++ {
			if code == 0xffff {
				break
			}
			glyph := uint16(0)
			if rangeOffset == 0 {
				glyph = uint16(code + uint32(delta))
			} else {
				position := idRangeOffsets + uint32(i)*2 + uint32(rangeOffset) + (code-uint32(start))*2
				glyph, err = readU16(data, table.offset, position)
				if err != nil {
					return err
				}
				if glyph != 0 {
					glyph = uint16(uint32(glyph) + uint32(delta))
				}
			}
			addGlyph(glyphs, code, glyph)
		}
	}
	return nil
}

func parseCmap6(data []byte, table tableRef, offset uint32, glyphs map[uint32]uint16) error {
	first, err := readU16(data, table.offset, offset+6)
	if err != nil {
		return err
	}
	count, err := readU16(data, table.offset, offset+8)
	if err != nil {
		return err
	}
	for i := 0; i < int(count); i++ {
		glyph, err := readU16(data, table.offset, offset+10+uint32(i)*2)
		if err != nil {
			return err
		}
		addGlyph(glyphs, uint32(first)+uint32(i), glyph)
	}
	return nil
}

func parseCmap12(data []byte, table tableRef, offset uint32, constant bool, glyphs map[uint32]uint16) error {
	groupCount, err := readU32(data, table.offset, offset+12)
	if err != nil {
		return err
	}
	if groupCount > 0x100000 {
		return errors.New("cmap format 12 has unreasonable group count")
	}
	for i := uint32(0); i < groupCount; i++ {
		record := offset + 16 + i*12
		start, err := readU32(data, table.offset, record)
		if err != nil {
			return err
		}
		end, err := readU32(data, table.offset, record+4)
		if err != nil {
			return err
		}
		startGlyph, err := readU32(data, table.offset, record+8)
		if err != nil {
			return err
		}
		if start > end || start > 0x10ffff {
			continue
		}
		if end > 0x10ffff {
			end = 0x10ffff
		}
		for code := start; code <= end; code++ {
			glyph := startGlyph
			if !constant {
				glyph = startGlyph + (code - start)
			}
			addGlyph(glyphs, code, uint16(glyph))
		}
	}
	return nil
}

func addGlyph(glyphs map[uint32]uint16, code uint32, glyph uint16) {
	if glyph == 0 || code > 0x10ffff {
		return
	}
	if _, exists := glyphs[code]; !exists {
		glyphs[code] = glyph
	}
}

package mxfindex

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	localTagInstanceUID   = 0x3c0a
	localTagEditUnitBytes = 0x3f05
	localTagIndexSID      = 0x3f06
	localTagBodySID       = 0x3f07
	localTagSliceCount    = 0x3f08
	localTagDeltaArray    = 0x3f09
	localTagEntryArray    = 0x3f0a
	localTagEditRate      = 0x3f0b
	localTagStartPosition = 0x3f0c
	localTagDuration      = 0x3f0d
	localTagTimecodeStart = 0x1501
	localTagTimecodeRate  = 0x1502
	localTagTimecodeDrop  = 0x1503
	maxLocalSetValue      = 256 << 20
)

func parseIndexSegment(r io.ReaderAt, p klvPacket, partitionIndex, index int) (IndexSegment, error) {
	var out IndexSegment
	out.Index = index
	out.Offset = p.Offset
	out.ValueLength = p.Length
	out.PartitionIndex = partitionIndex
	if p.Length > maxLocalSetValue {
		return out, fmt.Errorf("mxfindex: index segment at %d is too large (%d bytes)", p.Offset, p.Length)
	}
	data := make([]byte, p.Length)
	if _, err := readFullAt(r, data, p.ValueOffset); err != nil {
		return out, fmt.Errorf("mxfindex: read index segment at %d: %w", p.Offset, err)
	}
	err := walkLocalSet(data, func(tag uint16, value []byte) error {
		switch tag {
		case localTagInstanceUID:
			if len(value) != 16 {
				return fmt.Errorf("IndexTable InstanceUID has length %d", len(value))
			}
			copy(out.InstanceUID[:], value)
		case localTagEditRate:
			if len(value) != 8 {
				return fmt.Errorf("IndexEditRate has length %d", len(value))
			}
			out.EditRate = Rational{
				Num: int64(binary.BigEndian.Uint32(value[0:4])),
				Den: int64(binary.BigEndian.Uint32(value[4:8])),
			}
		case localTagStartPosition:
			if len(value) != 8 {
				return fmt.Errorf("IndexStartPosition has length %d", len(value))
			}
			out.StartPosition = binary.BigEndian.Uint64(value)
		case localTagDuration:
			if len(value) != 8 {
				return fmt.Errorf("IndexDuration has length %d", len(value))
			}
			out.Duration = binary.BigEndian.Uint64(value)
		case localTagEditUnitBytes:
			if len(value) != 4 {
				return fmt.Errorf("EditUnitByteCount has length %d", len(value))
			}
			out.EditUnitByteCount = binary.BigEndian.Uint32(value)
		case localTagIndexSID:
			if len(value) != 4 {
				return fmt.Errorf("IndexSID has length %d", len(value))
			}
			out.IndexSID = binary.BigEndian.Uint32(value)
		case localTagBodySID:
			if len(value) != 4 {
				return fmt.Errorf("BodySID has length %d", len(value))
			}
			out.BodySID = binary.BigEndian.Uint32(value)
		case localTagSliceCount:
			if len(value) != 1 {
				return fmt.Errorf("SliceCount has length %d", len(value))
			}
			out.SliceCount = value[0]
		case localTagDeltaArray:
			deltas, err := parseDeltaArray(value)
			if err != nil {
				return err
			}
			out.DeltaEntries = deltas
		case localTagEntryArray:
			entries, err := parseIndexEntries(value)
			if err != nil {
				return err
			}
			out.HasEntryArray = true
			out.Entries = entries
		}
		return nil
	})
	if err != nil {
		out.Malformed = true
		return out, fmt.Errorf("mxfindex: malformed index segment at %d: %w", p.Offset, err)
	}
	return out, nil
}

func walkLocalSet(data []byte, fn func(tag uint16, value []byte) error) error {
	for pos := 0; pos < len(data); {
		if pos+4 > len(data) {
			return fmt.Errorf("local set has %d trailing byte(s)", len(data)-pos)
		}
		tag := binary.BigEndian.Uint16(data[pos : pos+2])
		size := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		pos += 4
		if size > len(data)-pos {
			return fmt.Errorf("local tag %#x extends past end of set", tag)
		}
		if size > 0 {
			if err := fn(tag, data[pos:pos+size]); err != nil {
				return fmt.Errorf("local tag %#x: %w", tag, err)
			}
		}
		pos += size
	}
	return nil
}

func parseDeltaArray(value []byte) ([]DeltaEntry, error) {
	if len(value) < 8 {
		return nil, fmt.Errorf("DeltaEntryArray has length %d", len(value))
	}
	count := binary.BigEndian.Uint32(value[0:4])
	size := binary.BigEndian.Uint32(value[4:8])
	if size < 6 || size > 1024 {
		return nil, fmt.Errorf("DeltaEntryArray entry size %d is invalid", size)
	}
	if uint64(count)*uint64(size) > uint64(len(value)-8) {
		return nil, fmt.Errorf("DeltaEntryArray count %d does not fit value", count)
	}
	out := make([]DeltaEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		start := 8 + int(i)*int(size)
		entry := DeltaEntry{
			Position:     int8(value[start]),
			Slice:        int8(value[start+1]),
			ElementDelta: binary.BigEndian.Uint32(value[start+2 : start+6]),
		}
		out = append(out, entry)
	}
	return out, nil
}

func parseIndexEntries(value []byte) ([]IndexEntry, error) {
	if len(value) < 8 {
		return nil, fmt.Errorf("IndexEntryArray has length %d", len(value))
	}
	count := binary.BigEndian.Uint32(value[0:4])
	size := binary.BigEndian.Uint32(value[4:8])
	if size < 11 || size > 1024 {
		return nil, fmt.Errorf("IndexEntryArray entry size %d is invalid", size)
	}
	if uint64(count)*uint64(size) > uint64(len(value)-8) {
		return nil, fmt.Errorf("IndexEntryArray count %d does not fit value", count)
	}
	out := make([]IndexEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		start := 8 + int(i)*int(size)
		entry := IndexEntry{
			TemporalOffset: int8(value[start]),
			KeyFrameOffset: int8(value[start+1]),
			Flags:          value[start+2],
			StreamOffset:   binary.BigEndian.Uint64(value[start+3 : start+11]),
		}
		if size >= 15 {
			entry.SliceOffset = binary.BigEndian.Uint32(value[start+11 : start+15])
		}
		out = append(out, entry)
	}
	return out, nil
}

func parseTimecodeComponent(r io.ReaderAt, p klvPacket) (TimecodeComponent, error) {
	var out TimecodeComponent
	out.Offset = p.Offset
	if p.Length > maxLocalSetValue {
		return out, fmt.Errorf("mxfindex: timecode component at %d is too large", p.Offset)
	}
	data := make([]byte, p.Length)
	if _, err := readFullAt(r, data, p.ValueOffset); err != nil {
		return out, fmt.Errorf("mxfindex: read timecode component at %d: %w", p.Offset, err)
	}
	err := walkLocalSet(data, func(tag uint16, value []byte) error {
		switch tag {
		case localTagInstanceUID:
			if len(value) != 16 {
				return fmt.Errorf("InstanceUID has length %d", len(value))
			}
			copy(out.InstanceUID[:], value)
		case localTagTimecodeStart:
			if len(value) != 8 {
				return fmt.Errorf("StartTimecode has length %d", len(value))
			}
			out.StartFrame = int64(binary.BigEndian.Uint64(value))
		case localTagTimecodeRate:
			if len(value) != 2 {
				return fmt.Errorf("RoundedTimecodeBase has length %d", len(value))
			}
			out.Rate = int(binary.BigEndian.Uint16(value))
		case localTagTimecodeDrop:
			if len(value) != 1 {
				return fmt.Errorf("DropFrame has length %d", len(value))
			}
			out.DropFrame = value[0] != 0
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("mxfindex: malformed timecode component at %d: %w", p.Offset, err)
	}
	if out.Rate > 0 {
		out.Timecode = FrameToTimecode(out.StartFrame, out.Rate, out.DropFrame)
	}
	return out, nil
}

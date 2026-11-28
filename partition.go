package mxfindex

import (
	"encoding/binary"
	"fmt"
	"io"
)

func parsePartition(r io.ReaderAt, p klvPacket, index int, runIn int64) (Partition, error) {
	var out Partition
	out.Index = index
	out.Offset = p.Offset
	out.TypeCode = p.Key[13]
	switch p.Key[13] {
	case 2:
		out.Type = "header"
	case 3:
		out.Type = "body"
	case 4:
		out.Type = "footer"
	default:
		return out, fmt.Errorf("mxfindex: unknown partition type %d", p.Key[13])
	}
	out.Closed = out.Type == "footer" || p.Key[14]&1 == 0
	out.Complete = p.Key[14] > 2
	if p.Length < 88 {
		return out, fmt.Errorf("mxfindex: partition pack at %d is too short (%d bytes)", p.Offset, p.Length)
	}
	data := make([]byte, p.Length)
	if _, err := readFullAt(r, data, p.ValueOffset); err != nil {
		return out, fmt.Errorf("mxfindex: read partition pack at %d: %w", p.Offset, err)
	}
	out.KAGSize = binary.BigEndian.Uint32(data[4:8])
	out.ThisPartition = binary.BigEndian.Uint64(data[8:16])
	out.PreviousPartition = binary.BigEndian.Uint64(data[16:24])
	out.FooterPartition = binary.BigEndian.Uint64(data[24:32])
	out.HeaderByteCount = binary.BigEndian.Uint64(data[32:40])
	out.IndexByteCount = binary.BigEndian.Uint64(data[40:48])
	out.IndexSID = binary.BigEndian.Uint32(data[48:52])
	out.BodyOffset = binary.BigEndian.Uint64(data[52:60])
	out.BodySID = binary.BigEndian.Uint32(data[60:64])
	copy(out.OperationalPattern[:], data[64:80])
	containerCount := binary.BigEndian.Uint32(data[80:84])
	itemSize := binary.BigEndian.Uint32(data[84:88])
	if containerCount > 1024 {
		return out, fmt.Errorf("mxfindex: implausible essence container count %d", containerCount)
	}
	if itemSize < 16 || itemSize > 1024 {
		return out, fmt.Errorf("mxfindex: invalid essence container item size %d", itemSize)
	}
	needed := int64(88) + int64(containerCount)*int64(itemSize)
	if needed > p.Length {
		return out, fmt.Errorf("mxfindex: partition pack at %d truncated in essence container list", p.Offset)
	}
	out.EssenceContainers = make([]UID, 0, containerCount)
	for i := uint32(0); i < containerCount; i++ {
		var uid UID
		start := 88 + int(i)*int(itemSize)
		copy(uid[:], data[start:start+16])
		out.EssenceContainers = append(out.EssenceContainers, uid)
	}
	if gotten := uint64(p.Offset - runIn); out.ThisPartition != gotten {
		return out, fmt.Errorf("mxfindex: partition at %d declares ThisPartition=%d, expected %d",
			p.Offset, out.ThisPartition, gotten)
	}
	return out, nil
}

func parseRandomIndexPack(r io.ReaderAt, p klvPacket) (RandomIndexPack, error) {
	var out RandomIndexPack
	out.Offset = p.Offset
	out.ValueLength = p.Length
	if p.Length < 4 || (p.Length-4)%12 != 0 {
		return out, fmt.Errorf("mxfindex: invalid RIP value length %d at %d", p.Length, p.Offset)
	}
	data := make([]byte, p.Length)
	if _, err := readFullAt(r, data, p.ValueOffset); err != nil {
		return out, fmt.Errorf("mxfindex: read RIP at %d: %w", p.Offset, err)
	}
	out.TotalLength = binary.BigEndian.Uint32(data[len(data)-4:])
	if int64(out.TotalLength) != p.totalLength() {
		return out, fmt.Errorf("mxfindex: RIP total length %d does not match KLV size %d",
			out.TotalLength, p.totalLength())
	}
	entryBytes := len(data) - 4
	out.Entries = make([]RIPEntry, 0, entryBytes/12)
	for offset := 0; offset < entryBytes; offset += 12 {
		out.Entries = append(out.Entries, RIPEntry{
			BodySID:    binary.BigEndian.Uint32(data[offset : offset+4]),
			ByteOffset: binary.BigEndian.Uint64(data[offset+4 : offset+12]),
		})
	}
	if len(out.Entries) > 0 {
		last := out.Entries[len(out.Entries)-1]
		if last.BodySID == 0 {
			out.FooterPartition = last.ByteOffset
		}
	}
	out.Valid = true
	return out, nil
}

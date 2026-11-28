package mxfindex

import (
	"bytes"
	"encoding/binary"
	"testing"
)

const (
	testRunIn = 32
)

var (
	testOP1aUL = UID{0x06, 0x0e, 0x2b, 0x34, 0x04, 0x01, 0x01, 0x01, 0x0d, 0x01, 0x02, 0x01, 0x01, 0x01, 0x09, 0x00}
	testECUL   = UID{0x06, 0x0e, 0x2b, 0x34, 0x04, 0x01, 0x01, 0x01, 0x0d, 0x01, 0x03, 0x01, 0x02, 0x06, 0x03, 0x00}
)

type fixtureBuilder struct {
	data []byte
}

func newFixtureBuilder() *fixtureBuilder {
	buf := make([]byte, testRunIn)
	for i := range buf {
		buf[i] = byte(i)
	}
	return &fixtureBuilder{data: buf}
}

func (b *fixtureBuilder) addKLV(key UID, value []byte) int64 {
	offset := int64(len(b.data))
	b.data = append(b.data, key[:]...)
	b.data = append(b.data, 0x83, byte(len(value)>>16), byte(len(value)>>8), byte(len(value)))
	b.data = append(b.data, value...)
	return offset
}

func (b *fixtureBuilder) patchU64(klvOffset, valueOffset int64, value uint64) {
	start := klvOffset + 20 + valueOffset
	binary.BigEndian.PutUint64(b.data[start:start+8], value)
}

func partitionValue(partType byte, bodySID, indexSID uint32) []byte {
	value := make([]byte, 104)
	binary.BigEndian.PutUint16(value[0:2], 1)
	binary.BigEndian.PutUint16(value[2:4], 3)
	binary.BigEndian.PutUint32(value[4:8], 512)
	binary.BigEndian.PutUint32(value[48:52], indexSID)
	binary.BigEndian.PutUint32(value[60:64], bodySID)
	copy(value[64:80], testOP1aUL[:])
	binary.BigEndian.PutUint32(value[80:84], 1)
	binary.BigEndian.PutUint32(value[84:88], 16)
	copy(value[88:104], testECUL[:])
	return value
}

func partitionKey(partType byte) UID {
	key := partitionKeyPrefix
	key[13] = partType
	key[14] = 4
	return key
}

func localTag(tag uint16, value []byte) []byte {
	out := make([]byte, 4+len(value))
	binary.BigEndian.PutUint16(out[0:2], tag)
	binary.BigEndian.PutUint16(out[2:4], uint16(len(value)))
	copy(out[4:], value)
	return out
}

func timecodeValue(start int64, rate uint16, drop bool) []byte {
	var out []byte
	out = append(out, localTag(localTagInstanceUID, bytes.Repeat([]byte{0x11}, 16))...)
	v8 := make([]byte, 8)
	binary.BigEndian.PutUint64(v8, uint64(start))
	out = append(out, localTag(localTagTimecodeStart, v8)...)
	v2 := make([]byte, 2)
	binary.BigEndian.PutUint16(v2, rate)
	out = append(out, localTag(localTagTimecodeRate, v2)...)
	d := byte(0)
	if drop {
		d = 1
	}
	out = append(out, localTag(localTagTimecodeDrop, []byte{d})...)
	return out
}

func indexValue(start, duration uint64, rate Rational, eubc uint32, entries []IndexEntry) []byte {
	var out []byte
	out = append(out, localTag(localTagInstanceUID, bytes.Repeat([]byte{0x22}, 16))...)
	v8 := make([]byte, 8)
	binary.BigEndian.PutUint32(v8[0:4], uint32(rate.Num))
	binary.BigEndian.PutUint32(v8[4:8], uint32(rate.Den))
	out = append(out, localTag(localTagEditRate, v8)...)
	binary.BigEndian.PutUint64(v8, start)
	out = append(out, localTag(localTagStartPosition, v8)...)
	binary.BigEndian.PutUint64(v8, duration)
	out = append(out, localTag(localTagDuration, v8)...)
	v4 := make([]byte, 4)
	binary.BigEndian.PutUint32(v4, eubc)
	out = append(out, localTag(localTagEditUnitBytes, v4)...)
	binary.BigEndian.PutUint32(v4, 2)
	out = append(out, localTag(localTagIndexSID, v4)...)
	binary.BigEndian.PutUint32(v4, 1)
	out = append(out, localTag(localTagBodySID, v4)...)
	out = append(out, localTag(localTagSliceCount, []byte{1})...)

	delta := make([]byte, 14)
	binary.BigEndian.PutUint32(delta[0:4], 1)
	binary.BigEndian.PutUint32(delta[4:8], 6)
	out = append(out, localTag(localTagDeltaArray, delta)...)
	if len(entries) > 0 {
		array := make([]byte, 8+len(entries)*15)
		binary.BigEndian.PutUint32(array[0:4], uint32(len(entries)))
		binary.BigEndian.PutUint32(array[4:8], 15)
		for i, entry := range entries {
			at := 8 + i*15
			array[at] = byte(entry.TemporalOffset)
			array[at+1] = byte(entry.KeyFrameOffset)
			array[at+2] = entry.Flags
			binary.BigEndian.PutUint64(array[at+3:at+11], entry.StreamOffset)
			binary.BigEndian.PutUint32(array[at+11:at+15], entry.SliceOffset)
		}
		out = append(out, localTag(localTagEntryArray, array)...)
	}
	return out
}

type fixtureOptions struct {
	noIndex       bool
	entryCount    int
	indexDuration uint64
	bodyOffset    uint64
	badBodyThis   bool
	cbr           bool
}

func buildFixture(t *testing.T, opts fixtureOptions) []byte {
	t.Helper()
	b := newFixtureBuilder()
	header := b.addKLV(partitionKey(2), partitionValue(2, 0, 0))
	b.addKLV(timecodeKey, timecodeValue(0, 25, false))
	body := b.addKLV(partitionKey(3), partitionValue(3, 1, 0))
	bodyStart := int64(len(b.data))
	var frame1 int64
	if opts.cbr {
		b.addKLV(UID{0x06, 0x0e, 0x2b, 0x34, 0x01, 0x02, 0x01, 0x01, 0x0d, 0x01, 0x03, 0x01, 0x15, 0x01, 0x05, 0x00}, bytes.Repeat([]byte{0x31}, 200))
	} else {
		b.addKLV(UID{0x06, 0x0e, 0x2b, 0x34, 0x01, 0x02, 0x01, 0x01, 0x0d, 0x01, 0x03, 0x01, 0x15, 0x01, 0x05, 0x00}, bytes.Repeat([]byte{0x31}, 120))
		b.addKLV(UID{0x06, 0x0e, 0x2b, 0x34, 0x01, 0x02, 0x01, 0x01, 0x0d, 0x01, 0x03, 0x01, 0x16, 0x01, 0x05, 0x00}, bytes.Repeat([]byte{0x32}, 40))
		frame1 = b.addKLV(UID{0x06, 0x0e, 0x2b, 0x34, 0x01, 0x02, 0x01, 0x01, 0x0d, 0x01, 0x03, 0x01, 0x15, 0x01, 0x05, 0x00}, bytes.Repeat([]byte{0x33}, 110))
		b.addKLV(UID{0x06, 0x0e, 0x2b, 0x34, 0x01, 0x02, 0x01, 0x01, 0x0d, 0x01, 0x03, 0x01, 0x16, 0x01, 0x05, 0x00}, bytes.Repeat([]byte{0x34}, 40))
	}
	footer := b.addKLV(partitionKey(4), partitionValue(4, 0, 2))

	entryCount := opts.entryCount
	if opts.cbr {
		entryCount = 0
	} else if entryCount == 0 {
		entryCount = 2
	}
	entries := make([]IndexEntry, 0, entryCount)
	for i := 0; i < entryCount; i++ {
		off := uint64(0)
		if i == 1 {
			off = uint64(frame1 - bodyStart)
		}
		entries = append(entries, IndexEntry{StreamOffset: off, Flags: 0xc0})
	}
	indexOff := int64(0)
	if !opts.noIndex {
		duration := opts.indexDuration
		if duration == 0 {
			duration = 2
		}
		eubc := uint32(0)
		if opts.cbr {
			eubc = 100
		}
		indexOff = b.addKLV(indexSegmentKey, indexValue(0, duration, Rational{Num: 25, Den: 1}, eubc, entries))
	}

	bodyRel := uint64(body - testRunIn)
	footerRel := uint64(footer - testRunIn)
	b.patchU64(header, 24, footerRel)
	b.patchU64(body, 8, bodyRel)
	b.patchU64(body, 16, 0)
	if opts.badBodyThis {
		b.patchU64(body, 8, bodyRel+1)
	}
	b.patchU64(body, 24, footerRel)
	b.patchU64(body, 52, opts.bodyOffset)
	b.patchU64(footer, 8, footerRel)
	b.patchU64(footer, 16, bodyRel)
	b.patchU64(footer, 24, footerRel)

	_ = indexOff
	ripValue := make([]byte, 0, 40)
	for _, entry := range []RIPEntry{{BodySID: 0, ByteOffset: 0}, {BodySID: 1, ByteOffset: bodyRel}, {BodySID: 0, ByteOffset: footerRel}} {
		chunk := make([]byte, 12)
		binary.BigEndian.PutUint32(chunk[0:4], entry.BodySID)
		binary.BigEndian.PutUint64(chunk[4:12], entry.ByteOffset)
		ripValue = append(ripValue, chunk...)
	}
	ripValue = append(ripValue, 0, 0, 0, 0)
	totalLen := 16 + 4 + len(ripValue)
	binary.BigEndian.PutUint32(ripValue[len(ripValue)-4:], uint32(totalLen))
	b.addKLV(randomIndexPackKey, ripValue)
	return b.data
}

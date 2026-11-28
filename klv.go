package mxfindex

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var (
	partitionKeyPrefix = UID{0x06, 0x0e, 0x2b, 0x34, 0x02, 0x05, 0x01, 0x01, 0x0d, 0x01, 0x02, 0x01, 0x01, 0x02}
	headerKeyPrefix    = partitionKeyPrefix
	indexSegmentKey    = UID{0x06, 0x0e, 0x2b, 0x34, 0x02, 0x53, 0x01, 0x01, 0x0d, 0x01, 0x02, 0x01, 0x01, 0x10, 0x01, 0x00}
	timecodeKey        = UID{0x06, 0x0e, 0x2b, 0x34, 0x02, 0x53, 0x01, 0x01, 0x0d, 0x01, 0x01, 0x01, 0x01, 0x01, 0x14, 0x00}
	randomIndexPackKey = UID{0x06, 0x0e, 0x2b, 0x34, 0x02, 0x05, 0x01, 0x01, 0x0d, 0x01, 0x02, 0x01, 0x01, 0x11, 0x01, 0x00}
	klvFillKey         = UID{0x06, 0x0e, 0x2b, 0x34, 0x01, 0x01, 0x01, 0x02, 0x03, 0x01, 0x02, 0x10, 0x01, 0x00, 0x00, 0x00}

	essenceKeyPrefixes = [][]byte{
		{0x06, 0x0e, 0x2b, 0x34, 0x01, 0x02, 0x01, 0x01, 0x0d, 0x01, 0x03, 0x01},
		{0x06, 0x0e, 0x2b, 0x34, 0x01, 0x02, 0x01, 0x01, 0x0e, 0x04, 0x03, 0x01},
		{0x06, 0x0e, 0x2b, 0x34, 0x01, 0x02, 0x01, 0x0a, 0x0e, 0x0f, 0x03, 0x01},
	}
	systemItemPrefixes = [][]byte{
		{0x06, 0x0e, 0x2b, 0x34, 0x02, 0x05, 0x01, 0x01, 0x0d, 0x01, 0x03, 0x01, 0x04},
		{0x06, 0x0e, 0x2b, 0x34, 0x02, 0x53, 0x01, 0x01, 0x0d, 0x01, 0x03, 0x01, 0x14},
	}
)

type klvPacket struct {
	Offset      int64
	Key         UID
	Length      int64
	ValueOffset int64
	End         int64
	LengthBytes int
}

func (p klvPacket) totalLength() int64 {
	return p.End - p.Offset
}

func readKLVAt(r io.ReaderAt, size, offset int64) (klvPacket, error) {
	var p klvPacket
	if offset < 0 || offset >= size {
		return p, fmt.Errorf("mxfindex: KLV offset %d outside file", offset)
	}
	key := make([]byte, 16)
	if _, err := readFullAt(r, key, offset); err != nil {
		return p, fmt.Errorf("mxfindex: read KLV key at %d: %w", offset, err)
	}
	copy(p.Key[:], key)
	p.Offset = offset
	length, lengthBytes, err := readBERLength(r, size, offset+16)
	if err != nil {
		return p, err
	}
	p.Length = length
	p.LengthBytes = lengthBytes
	p.ValueOffset = offset + 16 + int64(lengthBytes)
	if length < 0 || p.ValueOffset > size || length > size-p.ValueOffset {
		return p, fmt.Errorf("mxfindex: KLV at %d has invalid length %d", offset, length)
	}
	p.End = p.ValueOffset + length
	return p, nil
}

func readBERLength(r io.ReaderAt, size, offset int64) (int64, int, error) {
	if offset >= size {
		return 0, 0, errors.New("mxfindex: missing BER length")
	}
	first := make([]byte, 1)
	if _, err := readFullAt(r, first, offset); err != nil {
		return 0, 0, fmt.Errorf("mxfindex: read BER length: %w", err)
	}
	if first[0]&0x80 == 0 {
		return int64(first[0]), 1, nil
	}
	count := int(first[0] & 0x7f)
	if count == 0 {
		return 0, 0, errors.New("mxfindex: indefinite BER length is not allowed in MXF")
	}
	if count > 8 {
		return 0, 0, fmt.Errorf("mxfindex: BER length uses %d bytes", count)
	}
	if offset+1+int64(count) > size {
		return 0, 0, errors.New("mxfindex: truncated BER length")
	}
	buf := make([]byte, count)
	if _, err := readFullAt(r, buf, offset+1); err != nil {
		return 0, 0, fmt.Errorf("mxfindex: read BER length: %w", err)
	}
	var length uint64
	for _, b := range buf {
		if length > (1<<56)-1 {
			return 0, 0, errors.New("mxfindex: BER length overflows int64")
		}
		length = length<<8 | uint64(b)
	}
	if length > uint64(1<<63-1) {
		return 0, 0, errors.New("mxfindex: BER length overflows int64")
	}
	return int64(length), count + 1, nil
}

func readFullAt(r io.ReaderAt, dst []byte, offset int64) (int, error) {
	n, err := r.ReadAt(dst, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, err
	}
	if n != len(dst) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return n, err
	}
	return n, nil
}

// findRunIn locates the first header partition. MXF permits arbitrary run-in
// bytes before the first KLV; all partition offsets are relative to that point.
func findRunIn(r io.ReaderAt, size int64) (int64, error) {
	const chunkSize = 64 << 10
	var carry []byte
	for pos := int64(0); pos < size; pos += chunkSize {
		n := int64(chunkSize)
		if size-pos < n {
			n = size - pos
		}
		buf := make([]byte, n)
		if _, err := readFullAt(r, buf, pos); err != nil {
			return 0, fmt.Errorf("mxfindex: scan for header partition: %w", err)
		}
		window := append(carry, buf...)
		if idx := bytes.Index(window, headerKeyPrefix[:14]); idx >= 0 {
			return pos - int64(len(carry)) + int64(idx), nil
		}
		if len(window) > 13 {
			carry = append([]byte(nil), window[len(window)-13:]...)
		} else {
			carry = window
		}
	}
	return 0, errors.New("mxfindex: header partition pack not found")
}

func isPartitionKey(key UID) (byte, bool) {
	if !bytes.Equal(key[:13], partitionKeyPrefix[:13]) {
		return 0, false
	}
	switch key[13] {
	case 2, 3, 4:
		return key[13], true
	default:
		return 0, false
	}
}

func elementKind(key UID) (kind string, track uint32, ok bool) {
	for _, prefix := range essenceKeyPrefixes {
		if bytes.Equal(key[:len(prefix)], prefix) {
			return "essence", binary.BigEndian.Uint32(key[12:16]), true
		}
	}
	for _, prefix := range systemItemPrefixes {
		if bytes.Equal(key[:len(prefix)], prefix) {
			return "system", binary.BigEndian.Uint32(key[12:16]), true
		}
	}
	return "", 0, false
}

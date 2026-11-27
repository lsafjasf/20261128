package wavpack

import (
	"encoding/binary"
	"os"
	"testing"
)

func testBlock(index uint64, samples uint32, crc uint32, flags uint32, extra []MetadataSpec) []byte {
	meta := append([]MetadataSpec{
		{ID: metaWVBitstream, Payload: []byte{0xaa, 0xbb}},
	}, extra...)
	return buildBlock(index, samples, crc, flags, meta)
}

func testCorrection(index uint64, samples uint32, crc uint32, residual bool, checksum bool) []byte {
	var meta []MetadataSpec
	meta = append(meta, MetadataSpec{ID: metaShapeWeights, Payload: []byte{1, 2, 3, 4}})
	if residual {
		meta = append(meta, MetadataSpec{ID: metaWVCBitstream, Payload: []byte{0x11, 0x22, 0x33, 0x44}})
	}
	flags := testFlags
	if !checksum {
		flags &^= flagHasChecksum
	}
	return buildBlock(index, samples, crc, flags, meta)
}

type MetadataSpec struct {
	ID      byte
	Payload []byte
}

func buildBlock(index uint64, samples uint32, crc uint32, flags uint32, meta []MetadataSpec) []byte {
	header := make([]byte, HeaderSize)
	copy(header[:4], headerWord)
	binary.LittleEndian.PutUint16(header[8:10], 0x410)
	header[10] = byte(index >> 32)
	header[11] = 0
	binary.LittleEndian.PutUint32(header[12:16], uint32(samples))
	binary.LittleEndian.PutUint32(header[16:20], uint32(index))
	binary.LittleEndian.PutUint32(header[20:24], samples)
	binary.LittleEndian.PutUint32(header[24:28], flags)
	binary.LittleEndian.PutUint32(header[28:32], crc)

	block := append([]byte(nil), header...)
	for _, item := range meta {
		block = appendMetadata(block, item.ID, item.Payload)
	}

	if flags&flagHasChecksum != 0 {
		binary.LittleEndian.PutUint32(block[4:8], uint32(len(block)+4-8))
		var sum uint32 = ^uint32(0)
		for pos := 0; pos < len(block); pos += 2 {
			sum = sum*3 + uint32(binary.LittleEndian.Uint16(block[pos:pos+2]))
		}
		sum ^= sum >> 16
		block = appendMetadata(block, metaBlockSum, []byte{byte(sum), byte(sum >> 8)})
	}

	binary.LittleEndian.PutUint32(block[4:8], uint32(len(block)-8))
	return block
}

func appendMetadata(block []byte, id byte, payload []byte) []byte {
	actual := len(payload)
	stored := actual + actual&1
	rawID := id
	if stored != actual {
		rawID |= metaOddSize
	}
	storedWords := stored / 2
	if storedWords > 255 {
		rawID |= metaLarge
	}

	block = append(block, rawID)
	if storedWords > 255 {
		block = append(block, byte(stored>>1), byte(stored>>9), byte(stored>>17))
	} else {
		block = append(block, byte(storedWords))
	}
	block = append(block, payload...)
	if stored != actual {
		block = append(block, 0)
	}
	return block
}

func writeFile(t *testing.T, path string, blocks ...[]byte) {
	t.Helper()
	var data []byte
	for _, block := range blocks {
		data = append(data, block...)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

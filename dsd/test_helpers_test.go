package dsd

import (
	"bytes"
	"encoding/binary"
)

func le16(v uint16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, v)
	return buf
}

func le32(v uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, v)
	return buf
}

func le64(v uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, v)
	return buf
}

func be16(v uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, v)
	return buf
}

func be32(v uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, v)
	return buf
}

func be64(v uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, v)
	return buf
}

func cat(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

func makeDFFChunk(id string, payload []byte) []byte {
	data := cat([]byte(id), be64(uint64(len(payload))), payload)
	if len(payload)%2 == 1 {
		data = append(data, 0xee)
	}
	return data
}

func makeDFF(channelIDs []string, sampleRate uint32, audioPayloads ...[]byte) []byte {
	chnl := cat(be16(uint16(len(channelIDs))))
	for _, id := range channelIDs {
		chnl = append(chnl, []byte(id)...)
	}
	prop := cat([]byte("SND "),
		makeDFFChunk("FS  ", be32(sampleRate)),
		makeDFFChunk("CHNL", chnl),
		makeDFFChunk("CMPR", []byte("DSD \x0enot compressed")),
	)
	body := cat([]byte("DSD "),
		makeDFFChunk("FVER", be32(0x01050000)),
		makeDFFChunk("PROP", prop),
	)
	for _, payload := range audioPayloads {
		body = append(body, makeDFFChunk("DSD ", payload)...)
	}
	file := cat([]byte("FRM8"), be64(uint64(len(body))), body)
	if len(body)%2 == 1 {
		file = append(file, 0xcc)
	}
	return file
}

func makeDSF(channels, blockSize int, samplesPerChannel uint64, data []byte) []byte {
	const dataOffset = 80
	fileSize := dataOffset + 12 + len(data)
	fmtBody := cat(
		le32(1),
		le32(0),
		le32(2),
		le32(uint32(channels)),
		le32(2822400),
		le32(1),
		le64(samplesPerChannel),
		le32(uint32(blockSize)),
		le32(0),
	)
	header := cat(
		[]byte("DSD "),
		le64(28),
		le64(uint64(fileSize)),
		le64(0),
	)
	fmtChunk := cat([]byte("fmt "), le64(52), fmtBody)
	dataChunk := cat([]byte("data"), le64(uint64(12+len(data))), data)
	return cat(header, fmtChunk, dataChunk)
}

func channelValues(manifest *Manifest, data []byte) ([][]byte, error) {
	return manifest.SampleTable(bytes.NewReader(data))
}

func countPadding(manifest *Manifest, kind PaddingKind) int {
	count := 0
	for _, padding := range manifest.Padding {
		if padding.Kind == kind {
			count++
		}
	}
	return count
}

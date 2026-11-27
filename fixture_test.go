package mp4safe

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type fixtureSpec struct {
	Wide        bool
	VideoSizes  []uint32
	AudioSizes  []uint32
	VideoData   []byte
	AudioData   []byte
	UnknownData []byte
}

func makeFixture(spec fixtureSpec) []byte {
	if spec.UnknownData == nil {
		spec.UnknownData = []byte("opaque-private-extension-payload")
	}
	videoChunks := len(spec.VideoSizes)
	audioChunks := len(spec.AudioSizes)
	makeTracks := func(videoOffsets, audioOffsets []uint64) []byte {
		video := makeTrack(1, "vide", spec.VideoSizes, 1, videoChunks, videoOffsets, spec.Wide)
		audio := makeTrack(2, "soun", spec.AudioSizes, 1, audioChunks, audioOffsets, spec.Wide)
		return box("moov", append(append(video, audio...), uuidBox(
			[16]byte{'u', 's', 'e', 'r', 't', 'y', 'p', 'e', '0', '1', '0', '2', '0', '3', '0', '4'},
			[]byte("uuid-extension-bytes"),
		)...))
	}
	zeros := func(n int) []uint64 { return make([]uint64, n) }
	prefix := append(box("ftyp", []byte("isom\x00\x00\x02\x00isomiso2")),
		box("free", bytes.Repeat([]byte{0xa5}, 24))...)
	moov0 := makeTracks(zeros(videoChunks), zeros(audioChunks))
	between := box("free", bytes.Repeat([]byte{0x5a}, 16))
	mdatPayloadStart := uint64(len(prefix) + len(moov0) + len(between) + 8)
	videoOffsets := chunkOffsets(mdatPayloadStart, spec.VideoSizes)
	audioStart := mdatPayloadStart + uint64(len(spec.VideoData))
	audioOffsets := chunkOffsets(audioStart, spec.AudioSizes)
	moov := makeTracks(videoOffsets, audioOffsets)
	if len(moov) != len(moov0) {
		panic("fixture moov size changed")
	}
	out := append([]byte(nil), prefix...)
	out = append(out, moov...)
	out = append(out, between...)
	out = append(out, box("mdat", append(append([]byte(nil), spec.VideoData...), spec.AudioData...))...)
	out = append(out, box("Xtra", spec.UnknownData)...)
	out = append(out, box("free", bytes.Repeat([]byte{0x3c}, 40))...)
	return out
}

func chunkOffsets(base uint64, sizes []uint32) []uint64 {
	out := make([]uint64, len(sizes))
	cur := base
	for i, size := range sizes {
		out[i] = cur
		cur += uint64(size)
	}
	return out
}

func makeTrack(id uint32, handler string, sizes []uint32, samplesPerChunk, chunkCount int, offsets []uint64, wide bool) []byte {
	tkhdPayload := make([]byte, 16)
	binary.BigEndian.PutUint32(tkhdPayload[8:12], id)
	hdlrPayload := make([]byte, 8)
	copy(hdlrPayload[4:8], handler)
	stscPayload := make([]byte, 4)
	binary.BigEndian.PutUint32(stscPayload, uint32(chunkCount))
	for chunk := 1; chunk <= chunkCount; chunk++ {
		entry := make([]byte, 12)
		binary.BigEndian.PutUint32(entry[0:4], uint32(chunk))
		binary.BigEndian.PutUint32(entry[4:8], uint32(samplesPerChunk))
		binary.BigEndian.PutUint32(entry[8:12], 1)
		stscPayload = append(stscPayload, entry...)
	}
	stszPayload := make([]byte, 8)
	binary.BigEndian.PutUint32(stszPayload[4:8], uint32(len(sizes)))
	for _, size := range sizes {
		var v [4]byte
		binary.BigEndian.PutUint32(v[:], size)
		stszPayload = append(stszPayload, v[:]...)
	}
	chunkPayload := make([]byte, 4)
	binary.BigEndian.PutUint32(chunkPayload, uint32(len(offsets)))
	for _, off := range offsets {
		if wide {
			var v [8]byte
			binary.BigEndian.PutUint64(v[:], off)
			chunkPayload = append(chunkPayload, v[:]...)
		} else {
			var v [4]byte
			binary.BigEndian.PutUint32(v[:], uint32(off))
			chunkPayload = append(chunkPayload, v[:]...)
		}
	}
	chunkType := "stco"
	if wide {
		chunkType = "co64"
	}
	stbl := box("stbl", concat(
		fullBox("stsc", stscPayload), fullBox("stsz", stszPayload),
		fullBox(chunkType, chunkPayload), box("zzzz", []byte("nested-unknown-box")),
	))
	return box("trak", concat(
		fullBox("tkhd", tkhdPayload),
		box("mdia", concat(
			fullBox("hdlr", hdlrPayload),
			box("minf", stbl),
		)),
	))
}

func box(typ string, payload []byte) []byte {
	if len(typ) != 4 {
		panic(fmt.Sprintf("bad box type %q", typ))
	}
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[:4], uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}

func fullBox(typ string, payload []byte) []byte {
	return box(typ, append(make([]byte, 4), payload...))
}

func uuidBox(userType [16]byte, payload []byte) []byte {
	out := make([]byte, 8+16+len(payload))
	binary.BigEndian.PutUint32(out[:4], uint32(len(out)))
	copy(out[4:8], "uuid")
	copy(out[8:24], userType[:])
	copy(out[24:], payload)
	return out
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

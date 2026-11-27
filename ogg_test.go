package opusmap

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func oggPage(serial, sequence uint32, flags byte, granule uint64, packet []byte) []byte {
	var lacing []byte
	remaining := packet
	for len(remaining) >= 255 {
		lacing = append(lacing, 255)
		remaining = remaining[255:]
	}
	lacing = append(lacing, byte(len(remaining)))
	return oggPageRaw(serial, sequence, flags, granule, lacing, packet)
}

func oggPageRaw(
	serial, sequence uint32, flags byte, granule uint64,
	lacing, payload []byte,
) []byte {
	header := make([]byte, 27)
	copy(header, "OggS")
	header[5] = flags
	binary.LittleEndian.PutUint64(header[6:14], granule)
	binary.LittleEndian.PutUint32(header[14:18], serial)
	binary.LittleEndian.PutUint32(header[18:22], sequence)
	header[26] = byte(len(lacing))
	page := append(header, lacing...)
	page = append(page, payload...)
	crc := oggCRC(page)
	binary.LittleEndian.PutUint32(page[22:26], crc)
	return page
}

func oggStream(head, audio []byte) []byte {
	tags := append([]byte("OpusTags"), make([]byte, 8)...)
	var out []byte
	out = append(out, oggPage(42, 0, oggBOS, 0, head)...)
	out = append(out, oggPage(42, 1, 0, 0, tags)...)
	out = append(out, oggPage(42, 2, oggEOS, 960, audio)...)
	return out
}

func sampleAudio(streams int) []byte {
	var payload []byte
	for stream := 0; stream < streams-1; stream++ {
		payload = append(payload, selfCode0(1, true, []byte{byte(stream), 1})...)
	}
	return append(payload, regularCode0(1, false, []byte{7})...)
}

func TestScanValidFamily2Stream(t *testing.T) {
	head := testHead(6, 2, 3, 2, []byte{0, 1, 2, 3, 4, 255})
	report, err := Scan(bytes.NewReader(oggStream(head, sampleAudio(3))))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid() {
		t.Fatalf("unexpected issues: %v", report.Issues)
	}
	if len(report.Packets) != 1 || len(report.Packets[0].Streams) != 3 {
		t.Fatalf("packets=%+v", report.Packets)
	}
	if report.Packets[0].PacketSeq != 2 {
		t.Fatalf("audio packet seq = %d", report.Packets[0].PacketSeq)
	}
}

func TestScanIllegalMappingPointsAtHeaderPacket(t *testing.T) {
	head := testHead(6, 2, 2, 1, []byte{0, 1, 2, 3, 4, 5})
	report, err := Scan(bytes.NewReader(oggStream(head, sampleAudio(2))))
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid() {
		t.Fatal("expected illegal mapping issues")
	}
	seen := 0
	for _, issue := range report.Issues {
		if issue.Code != CodeIllegalMapping {
			continue
		}
		seen++
		if issue.PacketSeq != 0 {
			t.Errorf("mapping issue packet = %d, want 0", issue.PacketSeq)
		}
	}
	if seen != 3 {
		t.Fatalf("got %d illegal mapping issues, want 3: %v", seen, report.Issues)
	}
}

func TestScanMissingStreamPointsAtAudioPacket(t *testing.T) {
	head := testHead(6, 2, 3, 2, []byte{0, 1, 2, 3, 4, 255})
	audio := selfCode0(1, true, []byte{1, 2})
	report, err := Scan(bytes.NewReader(oggStream(head, audio)))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("issues = %v", report.Issues)
	}
	issue := report.Issues[0]
	if issue.Code != CodeMissingStream || issue.PacketSeq != 2 {
		t.Fatalf("issue = %+v", issue)
	}
}

func TestScanDurationMismatchPointsAtStream(t *testing.T) {
	head := testHead(6, 2, 3, 2, []byte{0, 1, 2, 3, 4, 255})
	audio := append(selfCode0(1, true, []byte{1}), selfCode0(2, true, []byte{2})...)
	audio = append(audio, regularCode0(1, false, []byte{3})...)
	report, err := Scan(bytes.NewReader(oggStream(head, audio)))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("issues = %v", report.Issues)
	}
	issue := report.Issues[0]
	if issue.Code != CodeDurationMismatch || issue.PacketSeq != 2 || issue.Stream != 1 {
		t.Fatalf("issue = %+v", issue)
	}
}

func TestScanAssemblyAcrossPages(t *testing.T) {
	head := testHead(6, 2, 3, 2, []byte{0, 1, 2, 3, 4, 255})
	tags := append([]byte("OpusTags"), make([]byte, 8)...)
	audio := append(selfCode0(1, true, make([]byte, 200)),
		selfCode0(1, false, make([]byte, 80))...)
	audio = append(audio, regularCode0(1, false, []byte{4, 5, 6})...)
	if len(audio) <= 255 {
		t.Fatalf("test packet is too short: %d", len(audio))
	}

	first := append([]byte(nil), audio[:255]...)
	second := append([]byte(nil), audio[255:]...)
	var stream []byte
	stream = append(stream, oggPage(7, 0, oggBOS, 0, head)...)
	stream = append(stream, oggPage(7, 1, 0, 0, tags)...)

	continuedPage := oggPageRaw(7, 2, 0, ^uint64(0), []byte{255}, first)
	stream = append(stream, continuedPage...)
	stream = append(stream, oggPage(7, 3, oggContinued|oggEOS, 960, second)...)

	report, err := Scan(bytes.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid() {
		t.Fatalf("issues: %v", report.Issues)
	}
	if len(report.Packets) != 1 || len(report.Packets[0].Streams) != 3 {
		t.Fatalf("packets=%+v", report.Packets)
	}
}

package opusmap

import (
	"strings"
	"testing"
)

func selfCode0(config byte, stereo bool, frame []byte) []byte {
	toc := config<<3 | 0
	if stereo {
		toc |= 0x04
	}
	out := []byte{toc, byte(len(frame))}
	return append(out, frame...)
}

func regularCode0(config byte, stereo bool, frame []byte) []byte {
	toc := config<<3 | 0
	if stereo {
		toc |= 0x04
	}
	return append([]byte{toc}, frame...)
}

func TestParseMultistreamPacketWithShortLastFrame(t *testing.T) {
	payload := append(selfCode0(1, true, []byte{1, 2, 3}), selfCode0(1, true, []byte{4})...)
	payload = append(payload, regularCode0(1, false, []byte{9})...)

	packets, err := ParseMultistreamPacket(payload, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(packets) != 3 {
		t.Fatalf("got %d streams, want 3", len(packets))
	}
	for i, packet := range packets {
		if packet.Duration48k != 960 {
			t.Errorf("stream %d duration = %d", i, packet.Duration48k)
		}
	}
	if packets[2].FrameLengths[0] != 1 {
		t.Fatalf("last frame length = %d, want 1", packets[2].FrameLengths[0])
	}
	if packets[2].SelfDelimited {
		t.Fatal("last packed stream must use regular framing")
	}
}

func TestParseMultistreamPacketDetectsMissingStream(t *testing.T) {
	payload := selfCode0(1, false, []byte{1})
	packets, err := ParseMultistreamPacket(payload, 3)
	if err == nil || !strings.Contains(err.Error(), "missing stream") {
		t.Fatalf("got %d packets, err %v", len(packets), err)
	}
	if len(packets) != 1 {
		t.Fatalf("parsed %d streams, want 1", len(packets))
	}
}

func TestParseSelfDelimitedVBRCode3(t *testing.T) {
	toc := byte(1<<3) | 3
	packet := []byte{
		toc, 0x82,
		2, 1,
		1, 2, 9,
	}
	parsed, err := parseOpusPacket(packet, true)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.FrameCount != 2 || parsed.Duration48k != 1920 {
		t.Fatalf("frames=%d duration=%d", parsed.FrameCount, parsed.Duration48k)
	}
	if parsed.Size != len(packet) ||
		parsed.FrameLengths[0] != 2 || parsed.FrameLengths[1] != 1 {
		t.Fatalf("size=%d lengths=%v", parsed.Size, parsed.FrameLengths)
	}
}

func TestParseRegularCode3WithPadding(t *testing.T) {
	toc := byte(1<<3) | 3
	packet := []byte{toc, 0x41, 2, 7, 8, 0, 0}
	parsed, err := parseOpusPacket(packet, false)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Duration48k != 960 {
		t.Fatalf("duration=%d", parsed.Duration48k)
	}
	if parsed.FrameLengths[0] != 2 {
		t.Fatalf("frame lengths=%v", parsed.FrameLengths)
	}
}

func TestParseSelfDelimitedCode1(t *testing.T) {
	packet := []byte{byte(1<<3) | 1, 2, 1, 2, 3, 4}
	parsed, err := parseOpusPacket(packet, true)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.FrameCount != 2 || parsed.Duration48k != 1920 {
		t.Fatalf("frames=%d duration=%d", parsed.FrameCount, parsed.Duration48k)
	}
	if parsed.Size != len(packet) || parsed.FrameLengths[0] != 2 ||
		parsed.FrameLengths[1] != 2 {
		t.Fatalf("size=%d lengths=%v", parsed.Size, parsed.FrameLengths)
	}
}

func TestParseSelfDelimitedCode2(t *testing.T) {
	packet := []byte{byte(1<<3) | 2, 1, 2, 9, 8, 7}
	parsed, err := parseOpusPacket(packet, true)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Size != len(packet) || parsed.FrameLengths[0] != 1 ||
		parsed.FrameLengths[1] != 2 {
		t.Fatalf("size=%d lengths=%v", parsed.Size, parsed.FrameLengths)
	}
}

func TestParseSelfDelimitedCBRCode3(t *testing.T) {
	packet := []byte{byte(1<<3) | 3, 0x02, 1, 8, 9}
	parsed, err := parseOpusPacket(packet, true)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.FrameCount != 2 || parsed.Size != len(packet) {
		t.Fatalf("frames=%d size=%d", parsed.FrameCount, parsed.Size)
	}
	if parsed.FrameLengths[0] != 1 || parsed.FrameLengths[1] != 1 {
		t.Fatalf("lengths=%v", parsed.FrameLengths)
	}
}

func TestParseZeroByteDTXFrame(t *testing.T) {
	parsed, err := parseOpusPacket([]byte{0}, false)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.FrameLengths[0] != 0 {
		t.Fatalf("frame length = %d", parsed.FrameLengths[0])
	}
}

package lc3

import (
	"encoding/binary"
	"testing"
)

func buildBitstream(t *testing.T, rate uint32, dur Duration, sizes []int, truncateLastBy int) []byte {
	t.Helper()
	hdr := make([]byte, 18)
	binary.LittleEndian.PutUint16(hdr[0:2], 0xCC1C)
	binary.LittleEndian.PutUint16(hdr[2:4], 18)
	binary.LittleEndian.PutUint16(hdr[4:6], uint16(rate/100))
	binary.LittleEndian.PutUint16(hdr[6:8], 320)
	binary.LittleEndian.PutUint16(hdr[8:10], 1)
	hdrMS := uint16(1000)
	if dur == Dur7p5ms {
		hdrMS = 750
	}
	binary.LittleEndian.PutUint16(hdr[10:12], hdrMS)
	binary.LittleEndian.PutUint32(hdr[14:18], 4800)

	var buf []byte
	buf = append(buf, hdr...)
	for i, size := range sizes {
		lenBuf := make([]byte, 2)
		binary.LittleEndian.PutUint16(lenBuf, uint16(size))
		buf = append(buf, lenBuf...)
		payload := make([]byte, size)
		if i == len(sizes)-1 && truncateLastBy > 0 {
			payload = payload[:size-truncateLastBy]
		}
		buf = append(buf, payload...)
	}
	return buf
}

func TestParseBitstreamHeader(t *testing.T) {
	raw := buildBitstream(t, 48000, Dur7p5ms, nil, 0)
	h, err := ParseBitstreamHeader(raw)
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	if h.Rate() != 48000 || h.Duration() != Dur7p5ms {
		t.Fatalf("decoded rate=%d dur=%v", h.Rate(), h.Duration())
	}
}

func TestParseBitstreamFrames(t *testing.T) {
	raw := buildBitstream(t, 32000, Dur10ms, []int{80, 90, 70}, 0)
	frames, err := ParseBitstreamFrames(raw)
	if err != nil {
		t.Fatalf("frames: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("got %d frames, want 3", len(frames))
	}
	for i, want := range []int{80, 90, 70} {
		if frames[i].Octets != want || len(frames[i].Payload) != want || frames[i].Truncated {
			t.Errorf("frame %d = %+v, want octets %d", i, frames[i], want)
		}
	}
}

func TestBitstreamShortTail(t *testing.T) {
	raw := buildBitstream(t, 16000, Dur10ms, []int{60, 60, 60}, 25)
	frames, err := ParseBitstreamFrames(raw)
	if err != nil {
		t.Fatalf("short tail should parse, got %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("got %d frames, want 3", len(frames))
	}
	last := frames[2]
	if !last.Truncated || last.Octets != 60 || len(last.Payload) != 35 {
		t.Errorf("tail frame not represented correctly: %+v", last)
	}
}

func TestBitstreamBadFileID(t *testing.T) {
	raw := buildBitstream(t, 16000, Dur10ms, nil, 0)
	raw[0] = 0x00
	if _, err := ParseBitstreamHeader(raw); err == nil {
		t.Error("expected error for bad file id")
	}
}

// TestBitstreamTimeline drives the reference bitstream into the timeline using
// the header's configured duration, proving the full path yields exact sample
// intervals and flags the truncated tail.
func TestBitstreamTimeline(t *testing.T) {
	raw := buildBitstream(t, 48000, Dur7p5ms, []int{90, 90}, 10)
	h, err := ParseBitstreamHeader(raw)
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	bs, err := ParseBitstreamFrames(raw)
	if err != nil {
		t.Fatalf("frames: %v", err)
	}
	frames := make([]Frame, 0, len(bs))
	for i, f := range bs {
		frames = append(frames, Frame{
			Seq:       uint16(i),
			Duration:  h.Duration(),
			Octets:    f.Octets,
			Payload:   f.Payload,
			Truncated: f.Truncated,
		})
	}
	tl, err := Build(h.Rate(), frames)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if tl.TotalSamples() != 2*360 {
		t.Errorf("total=%d, want 720", tl.TotalSamples())
	}
	if !tl.TailTruncated {
		t.Error("expected TailTruncated")
	}
}

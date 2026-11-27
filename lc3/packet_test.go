package lc3

import (
	"bytes"
	"testing"
)

func TestPacketRoundTrip(t *testing.T) {
	payload := bytes.Repeat([]byte{0xAB}, 100)
	p, err := Packetize(7, Dur7p5ms, []int{40, 60}, payload)
	if err != nil {
		t.Fatalf("Packetize: %v", err)
	}
	raw, err := p.MarshalBinary()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := ParsePacket(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Seq != 7 || got.Duration != Dur7p5ms {
		t.Fatalf("header mismatch seq=%d dur=%v", got.Seq, got.Duration)
	}
	if len(got.Frames) != 2 {
		t.Fatalf("frames=%d, want 2", len(got.Frames))
	}
	if got.Frames[0].Octets != 40 || got.Frames[1].Octets != 60 {
		t.Errorf("octets = %d/%d, want 40/60", got.Frames[0].Octets, got.Frames[1].Octets)
	}
	if !bytes.Equal(got.Frames[1].Payload, payload[40:]) {
		t.Error("second frame payload mismatch")
	}
	if got.Frames[0].Seq != 7 || got.Frames[1].Seq != 8 {
		t.Errorf("frame seqs = %d/%d, want 7/8", got.Frames[0].Seq, got.Frames[1].Seq)
	}
}

// TestTruncatedTailFrame feeds a packet whose final frame declares more octets
// than the buffer holds. The parser must keep the partial frame and flag it
// rather than crash or silently drop it.
func TestTruncatedTailFrame(t *testing.T) {
	p := Packet{
		Seq:      3,
		Duration: Dur10ms,
		Frames: []Frame{
			{Seq: 3, Duration: Dur10ms, Octets: 50, Payload: make([]byte, 50)},
			{Seq: 4, Duration: Dur10ms, Octets: 50, Payload: make([]byte, 20)}, // short
		},
	}
	raw, err := p.MarshalBinary()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := ParsePacket(raw)
	if err != nil {
		t.Fatalf("Parse should tolerate short tail, got %v", err)
	}
	if len(got.Frames) != 2 {
		t.Fatalf("frames=%d, want 2", len(got.Frames))
	}
	if !got.Frames[1].Truncated || len(got.Frames[1].Payload) != 20 {
		t.Errorf("tail not marked truncated: %+v", got.Frames[1])
	}
	if got.Frames[0].Truncated {
		t.Error("first frame should be complete")
	}

	frames, err := ParsePacketStream(raw)
	if err != nil {
		t.Fatalf("stream parse: %v", err)
	}
	tl, err := Build(48000, frames)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !tl.TailTruncated {
		t.Error("timeline should report TailTruncated")
	}
	// Even a truncated tail frame still occupies its full nominal sample span.
	if tl.TotalSamples() != 2*480 {
		t.Errorf("total=%d, want 960", tl.TotalSamples())
	}
}

func TestShortHeaderErrors(t *testing.T) {
	if _, err := ParsePacket([]byte{0, 1, 2}); err == nil {
		t.Error("expected ErrShortBuffer for tiny packet")
	}
}

// TestEndToEndModeSwitchAndLoss builds a multi-packet stream, drops a packet,
// re-parses the byte stream, and reconstructs an exact gap-aware timeline.
func TestEndToEndModeSwitchAndLoss(t *testing.T) {
	const rate = uint32(24000)
	p0, _ := Packetize(0, Dur10ms, []int{100, 100}, make([]byte, 200))
	p1, _ := Packetize(2, Dur10ms, []int{100, 100}, make([]byte, 200))
	p2, _ := Packetize(4, Dur7p5ms, []int{75, 75}, make([]byte, 150)) // switch to 7.5ms
	// p3 (seqs 6,7) is "lost" by omission.
	p4, _ := Packetize(8, Dur7p5ms, []int{75}, make([]byte, 75))

	raw, err := MarshalPackets([]Packet{p0, p1, p2, p4})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	frames, err := ParsePacketStream(raw)
	if err != nil {
		t.Fatalf("parse stream: %v", err)
	}
	tl, err := Build(rate, frames)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	// 7 received frames, 2 lost.
	if tl.Received != 7 || tl.Lost != 2 {
		t.Fatalf("received=%d lost=%d, want 7/2", tl.Received, tl.Lost)
	}
	gap := tl.Gaps()
	if len(gap) != 1 || gap[0].Seq != 6 || gap[0].Lost != 2 {
		t.Fatalf("gap = %+v, want single gap seq 6 lost 2", gap)
	}

	// Exact received sample span: 4 frames at 240 (10ms@24k) + 3 frames at
	// 180 (7.5ms@24k) = 960 + 540 = 1500; gap adds 2*180 = 360.
	wantTotal := uint64(4*240 + 3*180 + 2*180)
	if tl.TotalSamples() != wantTotal {
		t.Errorf("total=%d, want %d", tl.TotalSamples(), wantTotal)
	}

	// Verify the boundary frame at the mode switch has the correct interval.
	fr := tl.Frames()
	// Frames 0..3 are 10ms; frame index 4 is the first 7.5ms frame at seq 4.
	if fr[4].Duration != Dur7p5ms || fr[4].Start != 4*240 || fr[4].End != 4*240+180 {
		t.Errorf("mode-switch frame interval wrong: %+v", fr[4])
	}
}

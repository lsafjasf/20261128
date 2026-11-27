package st2110

import (
	"encoding/binary"
	"testing"
)

func buildRTP(seq uint16, ts uint32, payloadLen int) []byte {
	b := make([]byte, rtpHeaderMin+payloadLen)
	b[0] = 0x80 // v2
	b[1] = 96   // dynamic PT
	binary.BigEndian.PutUint16(b[2:4], seq)
	binary.BigEndian.PutUint32(b[4:8], ts)
	binary.BigEndian.PutUint32(b[8:12], 0xdeadbeef)
	for i := range b[rtpHeaderMin:] {
		b[rtpHeaderMin+i] = byte(i)
	}
	return b
}

func TestParsePacket(t *testing.T) {
	raw := buildRTP(7, 48000, 36)
	p, err := ParsePacket(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.SequenceNumber != 7 || p.Timestamp != 48000 || p.SSRC != 0xdeadbeef {
		t.Fatalf("bad header: %+v", p)
	}
	if len(p.Payload) != 36 {
		t.Fatalf("payload len %d", len(p.Payload))
	}
	if _, err := ParsePacket(raw[:5]); err != ErrShortPacket {
		t.Fatalf("want ErrShortPacket, got %v", err)
	}
	bad := append([]byte(nil), raw...)
	bad[0] = 0xC0 // version 3
	if _, err := ParsePacket(bad); err == nil {
		t.Fatal("want version error")
	}
}

func TestParsePacketPaddingAndExtension(t *testing.T) {
	raw := buildRTP(1, 0, 12)
	ext := make([]byte, 0, 64)
	hdr := append([]byte(nil), raw[:rtpHeaderMin]...)
	hdr[0] = 0xB0 // v2, padding, extension
	ext = append(ext, hdr...)
	ext = append(ext, 0x10, 0x00, 0x00, 0x01) // ext profile + 1x32bit
	ext = append(ext, 0xaa, 0xbb, 0xcc, 0xdd)
	ext = append(ext, raw[rtpHeaderMin:]...)
	ext = append(ext, 0x00, 0x00, 0x00, 0x04) // 4 bytes padding
	p, err := ParsePacket(ext)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Payload) != 12 {
		t.Fatalf("payload len %d, want 12", len(p.Payload))
	}
}

func TestSamplesInPayload(t *testing.T) {
	f := Format{Encoding: EncodingL24, Channels: 8, SampleRate: 48000}
	n, err := f.SamplesInPayload(48 * 24) // 48 frames * 8ch * 3B
	if err != nil || n != 48 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, err := f.SamplesInPayload(25); err == nil {
		t.Fatal("want frame alignment error")
	}
	if _, err := f.SamplesInPayload(0); err == nil {
		t.Fatal("want empty payload error")
	}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Format{Encoding: EncodingL16, Channels: 2, SampleRate: 44100}).Validate(); err == nil {
		t.Fatal("want sample rate error")
	}
}

func TestSeqTracker(t *testing.T) {
	tr := NewSeqTracker(8)
	cases := []struct {
		seq  uint16
		want Class
	}{
		{100, ClassInOrder},
		{101, ClassInOrder},
		{101, ClassDuplicate},
		{104, ClassInOrder}, // gap: 102,103 lost
		{103, ClassReordered},
		{102, ClassReordered},
		{102, ClassDuplicate},
		{90, ClassLate}, // 14 behind max, beyond window
	}
	for _, c := range cases {
		if got := tr.Classify(c.seq); got != c.want {
			t.Fatalf("seq %d: got %d want %d", c.seq, got, c.want)
		}
	}
	if tr.Lost() != 2 {
		t.Fatalf("lost=%d want 2", tr.Lost())
	}
}

func TestSeqTrackerWrap(t *testing.T) {
	tr := NewSeqTracker(8)
	for _, seq := range []uint16{65534, 65535, 0, 1} {
		if got := tr.Classify(seq); got != ClassInOrder {
			t.Fatalf("seq %d: got %d want InOrder", seq, got)
		}
	}
	if got := tr.Classify(65535); got != ClassDuplicate {
		t.Fatalf("wrapped reordered: got %d", got)
	}
	if got := tr.Classify(65533); got != ClassReordered {
		t.Fatalf("wrapped unseen reordered: got %d", got)
	}
	if tr.Lost() != 0 {
		t.Fatalf("lost=%d want 0", tr.Lost())
	}
}

func TestMergerDeduplicatesLegs(t *testing.T) {
	m := NewMerger(8)
	pkt := Packet{SequenceNumber: 500}
	if ok, _ := m.Push(LegA, pkt); !ok {
		t.Fatal("first copy must be accepted")
	}
	if ok, class := m.Push(LegB, pkt); ok || class != ClassDuplicate {
		t.Fatalf("second copy: ok=%v class=%d", ok, class)
	}
	// Leg A loses 501, leg B delivers it: seamless failover.
	pkt.SequenceNumber = 501
	if ok, _ := m.Push(LegB, pkt); !ok {
		t.Fatal("failover copy must be accepted")
	}
	if m.Lost() != 0 {
		t.Fatalf("lost=%d want 0", m.Lost())
	}
	if got := m.LegStats(LegB).Duplicate; got != 1 {
		t.Fatalf("leg B duplicates=%d want 1", got)
	}
}

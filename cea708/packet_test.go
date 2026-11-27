package cea708

import "testing"

func TestHeaderLen(t *testing.T) {
	cases := []struct {
		header     byte
		wantSeq    int
		wantLength int
	}{
		{0x02, 0, 3},   // size 2 -> 2*2-1 = 3 payload bytes
		{0x82, 2, 3},   // sequence bits honored, same length
		{0x01, 0, 1},   // size 1 -> 1 payload byte
		{0xC0, 3, 127}, // size 0 -> maximum 127 payload bytes
	}
	for _, tc := range cases {
		seq, length := headerLen(tc.header)
		if seq != tc.wantSeq || length != tc.wantLength {
			t.Fatalf("headerLen(%#02x) = (%d,%d), want (%d,%d)",
				tc.header, seq, length, tc.wantSeq, tc.wantLength)
		}
	}
}

func TestParsePacketServiceBlocks(t *testing.T) {
	// header says 3 payload bytes; one service block for service 1 containing
	// one G0 byte, plus no padding.
	packet := []byte{0x02, 0x21, 0x41, 0x00}
	_, blocks := ParsePacket(packet)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Number != 1 || len(blocks[0].Data) != 1 || blocks[0].Data[0] != 0x41 {
		t.Fatalf("unexpected block: %+v", blocks[0])
	}
}

func TestParsePacketExtendedService(t *testing.T) {
	// First header byte: service pattern 7 + block size 2; second byte gives
	// extended service number 8; then two G0 bytes 'A','B'.
	packet := []byte{0x03, 0xE2, 0x08, 0x41, 0x42}
	_, blocks := ParsePacket(packet)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 extended block, got %d", len(blocks))
	}
	if blocks[0].Number != 8 || string(blocks[0].Data) != "AB" {
		t.Fatalf("unexpected extended block: %+v", blocks[0])
	}
}

func TestPacketAssembler(t *testing.T) {
	var a PacketAssembler
	// Packet start triple (cc_type 3) with header 0x02 (3 payload bytes), then a
	// continuation triple (cc_type 2) carrying the remaining bytes.
	if pkt, ok := a.PushTriple(3, true, 0x02, 0x21); ok {
		t.Fatalf("unexpected early packet: %x", pkt)
	}
	pkt, ok := a.PushTriple(2, true, 0x41, 0x00)
	if !ok {
		t.Fatal("expected completed packet")
	}
	want := []byte{0x02, 0x21, 0x41, 0x00}
	if len(pkt) != len(want) {
		t.Fatalf("packet length %d, want %d", len(pkt), len(want))
	}
	for i := range want {
		if pkt[i] != want[i] {
			t.Fatalf("packet %x, want %x", pkt, want)
		}
	}
}

func TestPacketAssemblerIgnoresInvalidAnd608(t *testing.T) {
	var a PacketAssembler
	if _, ok := a.PushTriple(3, false, 0x02, 0x21); ok {
		t.Fatal("invalid triple must be ignored")
	}
	if _, ok := a.PushTriple(0, true, 0x41, 0x00); ok {
		t.Fatal("CEA-608 triple must be ignored")
	}
}

func TestParsePacketEmptyAndPadding(t *testing.T) {
	// A packet whose payload is only null service blocks yields no blocks.
	if _, blocks := ParsePacket([]byte{0x02, 0x00, 0x00, 0x00}); len(blocks) != 0 {
		t.Fatalf("padding-only packet should have no blocks, got %d", len(blocks))
	}
	// Empty input must not panic.
	if _, blocks := ParsePacket(nil); blocks != nil {
		t.Fatal("empty packet should have nil blocks")
	}
}

func TestAssemblerReset(t *testing.T) {
	var a PacketAssembler
	a.PushTriple(3, true, 0x05, 0x00)
	a.Reset()
	if _, ok := a.PushTriple(2, true, 0x41, 0x00); ok {
		t.Fatal("after reset a continuation triple must not complete a packet")
	}
}

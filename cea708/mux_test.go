package cea708

import "testing"

func TestMultiplexerRoutesServices(t *testing.T) {
	mux := NewMultiplexer()

	// Packet with two service blocks: service 1 writes 'A', service 2 'B'.
	packet := []byte{
		0x05,      // header: payload length 9 (5*2-1)
		0x21, 'A', // service 1, one byte
		0x41, 'B', // service 2, one byte
		0x00, 0x00, 0x00, 0x00, 0x00, // padding to fill payload
	}
	blocks := mux.PushPacket(packet)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 decoded blocks, got %d", len(blocks))
	}
	if blocks[0].Service != 1 || blocks[1].Service != 2 {
		t.Fatalf("service routing wrong: %+v", blocks)
	}
	first := chars(blocks[0].Tokens)
	second := chars(blocks[1].Tokens)
	if first != "A" || second != "B" {
		t.Fatalf("per-service text wrong: %q %q", first, second)
	}

	// State persists: the same service decoders are returned on later packets.
	if got := mux.Services(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("service order wrong: %v", got)
	}
}

func TestEndToEndMixedTables(t *testing.T) {
	// One continuous caption exercising every required feature:
	// DefineWindow + SetCurrentWindow, G0 text, G1 (Latin-1), G2 via EXT1 and
	// SS2, G3 [CC], a custom glyph, and an unknown reserved code.
	var a PacketAssembler
	serviceBlock := []byte{
		c1DF0, 0x00, 0x00, 0x00, 0x00, 0x0F, 0x00, // 1 row, 16 cols
		c1CW0,
		'A', 'B',
		0xE9,         // G1 é
		c0EXT1, 0x33, // G2 left double quote
		c0SS2, 0x35, // single-shift G2 bullet
		c0EXT1, 0xA0, // G3 [CC]
		c0EXT1, c3DefineCustom, 0x00, // custom glyph: target slot G0
		0x5A,       // referenced at graphic index 0x5A
		0x00,       // 1 row, 1 column
		0b10000000, // 1x1 bitmap (single lit pixel)
		0x5A,       // reference the custom glyph
		0x05,       // reserved C0 -> unknown, must not be rendered
	}

	blockLen := len(serviceBlock)
	// Build a DTVCC packet: one service-1 block header plus its caption bytes,
	// padded with null service blocks to the advertised payload length.
	payload := append([]byte{byte(0x20 | (blockLen & 0x1F))}, serviceBlock...)
	totalPayload := len(payload)
	headerSize := (totalPayload + 2) / 2 // header length field counts header in
	header := byte(headerSize)
	full := append([]byte{header}, payload...)
	for len(full) < headerSize*2 {
		full = append(full, 0x00)
	}
	if len(full) != 1+totalPayload+(headerSize*2-1-totalPayload) {
		t.Fatalf("test framing error: full=%d payload=%d", len(full), totalPayload)
	}

	// Feed it through the assembler as cc triples.
	var packet []byte
	var got bool
	for pos := 0; pos < len(full); pos += 2 {
		b1 := full[pos]
		var b2 byte
		if pos+1 < len(full) {
			b2 = full[pos+1]
		}
		ccType := 3
		if pos != 0 {
			ccType = 2
		}
		packet, got = a.PushTriple(ccType, true, b1, b2)
		if got {
			break
		}
	}
	if !got {
		t.Fatal("assembler never produced a packet")
	}

	mux := NewMultiplexer()
	decoded := mux.PushPacket(packet)
	if len(decoded) != 1 {
		t.Fatalf("expected one service block, got %d", len(decoded))
	}
	tokens := decoded[0].Tokens

	var unknown int
	var custom int
	var cc int
	for _, tok := range tokens {
		switch tok.Kind {
		case KindUnknown:
			unknown++
			if len(tok.Raw) == 0 {
				t.Fatal("unknown token lost its raw bytes")
			}
		case KindCustomChar:
			custom++
		case KindClosedCaption:
			cc++
		}
	}
	if unknown != 1 || custom != 1 || cc != 1 {
		t.Fatalf("token mix wrong: unknown=%d custom=%d cc=%d", unknown, custom, cc)
	}

	// The custom glyph must still be installed after the block decoded.
	if _, ok := mux.Service(1).CustomChar(G0, 0x5A); !ok {
		t.Fatal("custom glyph did not persist beyond its defining block")
	}
}

package cea708

import "testing"

// defineWindow0 emits a DefineWindow(0) command followed by SetCurrentWindow(0)
// so subsequent graphic bytes have somewhere to land.
func defineWindow0(d *ServiceDecoder) {
	df := []byte{c1DF0,
		0x00,       // priority/locks/visibility
		0x00, 0x00, // anchor vertical/horizontal
		0x00, 0x07, // anchor/row count 1, col count 8
		0x00, // styles
	}
	d.DecodeBlock(df)
	d.DecodeBlock([]byte{c1CW0})
}

func chars(tokens []Token) string {
	out := make([]rune, 0, len(tokens))
	for _, tok := range tokens {
		if tok.Kind == KindChar {
			out = append(out, tok.Rune)
		}
	}
	return string(out)
}

func TestG0BasicAndNote(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	tokens, _ := d.DecodeBlock([]byte{'H', 'i', 0x7F})
	if got := chars(tokens); got != "Hi♪" {
		t.Fatalf("G0 decode = %q, want Hi♪", got)
	}
	if tokens[2].Table != 0 || tokens[2].Rune != '♪' {
		t.Fatalf("musical note token wrong: %+v", tokens[2])
	}
	if got := d.CurrentWindow().Text(); got != "Hi♪" {
		t.Fatalf("window text = %q", got)
	}
}

func TestG1Latin1(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	// 0xE9 is 'é' in the default G1 (Latin-1) GR region.
	tokens, _ := d.DecodeBlock([]byte{0xE9, 0xA9})
	if len(tokens) != 2 || tokens[0].Kind != KindChar || tokens[0].Rune != 'é' {
		t.Fatalf("G1 decode wrong: %+v", tokens)
	}
	if tokens[0].Table != 1 {
		t.Fatalf("expected G1 table index, got %d", tokens[0].Table)
	}
	if tokens[1].Rune != '©' {
		t.Fatalf("second G1 rune = %q", tokens[1].Rune)
	}
}

func TestG2ViaEXT1(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	// EXT1 + 0x33 -> left double quotation mark.
	tokens, _ := d.DecodeBlock([]byte{c0EXT1, 0x33})
	if len(tokens) != 1 || tokens[0].Kind != KindChar || tokens[0].Rune != '“' {
		t.Fatalf("G2 via EXT1 wrong: %+v", tokens)
	}
	if len(tokens[0].Raw) != 2 || tokens[0].Raw[0] != c0EXT1 {
		t.Fatalf("G2 raw prefix not preserved: %x", tokens[0].Raw)
	}
}

func TestG2TransparentSpaces(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	tokens, _ := d.DecodeBlock([]byte{c0EXT1, 0x20, c0EXT1, 0x21})
	if tokens[0].Kind != KindTransparentSpace || tokens[1].Kind != KindNBTransparentSpace {
		t.Fatalf("transparent space kinds wrong: %+v", tokens)
	}
}

func TestG3ClosedCaptionSign(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	tokens, _ := d.DecodeBlock([]byte{c0EXT1, 0xA0})
	if len(tokens) != 1 || tokens[0].Kind != KindClosedCaption {
		t.Fatalf("G3 [CC] wrong: %+v", tokens)
	}
	if got := d.CurrentWindow().Text(); got != "[CC]" {
		t.Fatalf("window rendered [CC] as %q", got)
	}
}

func TestSingleShift(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	// SS2 then a GL byte (0x33) must be read from G2 once, then GL reverts to
	// G0 for the following byte 'A'.
	tokens, _ := d.DecodeBlock([]byte{c0SS2, 0x33, 'A'})
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[0].Table != 2 || tokens[0].Rune != '“' {
		t.Fatalf("SS2 did not invoke G2 once: %+v", tokens[0])
	}
	if tokens[1].Table != 0 || tokens[1].Rune != 'A' {
		t.Fatalf("GL did not revert to G0 after single shift: %+v", tokens[1])
	}
}

func TestLockingShiftAndTableSelect(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)

	// Designate G2 into the GL slot via the two-byte table-select operation is
	// not needed for G2 itself; instead lock GL to G2 and read 0x33.
	tokens, _ := d.DecodeBlock([]byte{c0LS2GL, 0x33, 0x33, c0LS0, 'A'})
	if tokens[0].Table != 2 || tokens[0].Rune != '“' {
		t.Fatalf("locking shift to G2 failed: %+v", tokens[0])
	}
	if tokens[1].Table != 2 || tokens[1].Rune != '“' {
		t.Fatalf("locking shift must persist across graphic bytes: %+v", tokens[1])
	}
	if tokens[2].Table != 0 || tokens[2].Rune != 'A' {
		t.Fatalf("LS0 did not restore G0: %+v", tokens[2])
	}

	// Table select: designate Latin-1 (page 1) into G0, then GL byte 0xE9 is
	// out of GL range; instead re-designate G1 and read 0xE9 through GR.
	d.DecodeBlock([]byte{c0TableSelect, 0x01}) // slot G0, page 1 (Latin-1)
	if d.designation[G0] != TableLatin1 {
		t.Fatalf("table select did not change G0 designation: %d", d.designation[G0])
	}
}

func TestUnknownCodePreservesRaw(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	// Reserved C0 code 0x05 and an unmapped G2 index must remain unknown with
	// their raw bytes intact.
	tokens, _ := d.DecodeBlock([]byte{0x05, c0EXT1, 0x40})
	if tokens[0].Kind != KindUnknown || len(tokens[0].Raw) != 1 || tokens[0].Raw[0] != 0x05 {
		t.Fatalf("reserved C0 not preserved: %+v", tokens[0])
	}
	if tokens[1].Kind != KindUnknown || len(tokens[1].Raw) != 2 {
		t.Fatalf("unknown G2 not preserved with prefix: %+v", tokens[1])
	}
	// Unknown graphic bytes must not be written into the window.
	if got := d.CurrentWindow().Text(); got != "" {
		t.Fatalf("unknown tokens leaked into window: %q", got)
	}
}

func TestP16(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	tokens, _ := d.DecodeBlock([]byte{c0P16, 0x4E, 0xAC})
	if len(tokens) != 1 || tokens[0].Kind != KindP16 || tokens[0].Value != 0x4EAC {
		t.Fatalf("P16 decode wrong: %+v", tokens)
	}
	if len(tokens[0].Raw) != 3 {
		t.Fatalf("P16 raw length = %d", len(tokens[0].Raw))
	}
}

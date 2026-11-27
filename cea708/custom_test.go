package cea708

import "testing"

// defineCustom builds a DefineCustomChar sequence installing a 2x3 glyph in
// the G0 slot at graphic index 'Z' (0x5A).
func defineCustom(d *ServiceDecoder) []byte {
	// b0: high nibble selects G0 (0); index 0x5A; rows 2 -> nibble 1, cols 3
	// -> nibble 2, so b2 = 0x12. Bitmap: 2 rows, stride 1 byte each.
	return []byte{
		c0EXT1, c3DefineCustom,
		0x00,       // target G0
		0x5A,       // graphic index 'Z'
		0x12,       // 2 rows, 3 cols
		0b11100000, // row 0: three lit pixels
		0b10100000, // row 1: first and third lit
	}
}

func TestCustomCharDefineAndReference(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)

	stream := defineCustom(d)
	_, cmds := d.DecodeBlock(stream)
	if len(cmds) != 1 || cmds[0].Kind != CmdDefineCustomChar {
		t.Fatalf("expected one define-custom command, got %+v", cmds)
	}
	glyph := cmds[0].Custom
	if glyph.Rows != 2 || glyph.Cols != 3 || !glyph.Pixel(0, 0) || glyph.Pixel(1, 1) {
		t.Fatalf("glyph geometry/pixels wrong: %+v", glyph)
	}
	if !glyph.Pixel(1, 2) {
		t.Fatalf("expected pixel (1,2) lit")
	}

	// Referencing index 0x5A through GL (default G0) yields a custom token.
	tokens, _ := d.DecodeBlock([]byte{0x5A})
	if len(tokens) != 1 || tokens[0].Kind != KindCustomChar || tokens[0].Value != 0x5A {
		t.Fatalf("custom reference wrong: %+v", tokens)
	}
	stored, ok := d.CustomChar(G0, 0x5A)
	if !ok || stored.Cols != 3 {
		t.Fatalf("custom glyph not retrievable: %+v ok=%v", stored, ok)
	}
}

func TestCustomCharLifetimeReset(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	d.DecodeBlock(defineCustom(d))
	if _, ok := d.CustomChar(G0, 0x5A); !ok {
		t.Fatal("glyph should exist after definition")
	}

	// A Reset command must remove downloaded glyphs.
	d.DecodeBlock([]byte{c1RST})
	if _, ok := d.CustomChar(G0, 0x5A); ok {
		t.Fatal("glyph must be cleared by Reset")
	}

	// After reset the same index resolves through the built-in G0 table again.
	defineWindow0(d)
	tokens, _ := d.DecodeBlock([]byte{0x5A})
	if tokens[0].Kind != KindChar || tokens[0].Rune != 'Z' {
		t.Fatalf("after reset index should resolve to built-in G0: %+v", tokens[0])
	}
}

func TestCustomCharRedefine(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	d.DecodeBlock(defineCustom(d))

	// Redefine the same slot/index with a 1x1 glyph.
	d.DecodeBlock([]byte{
		c0EXT1, c3DefineCustom,
		0x00, 0x5A,
		0x00, // 1 row, 1 col
		0b10000000,
	})
	glyph, ok := d.CustomChar(G0, 0x5A)
	if !ok || glyph.Rows != 1 || glyph.Cols != 1 {
		t.Fatalf("redefinition did not replace glyph: %+v ok=%v", glyph, ok)
	}
}

func TestTruncatedCustomNotInstalled(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	// Header announces a 2x3 glyph (6 bitmap bits) but no bitmap follows.
	d.DecodeBlock([]byte{c0EXT1, c3DefineCustom, 0x00, 0x5A, 0x12})
	if _, ok := d.CustomChar(G0, 0x5A); ok {
		t.Fatal("a truncated glyph must not be installed")
	}

	// A dangling header with even fewer bytes must not panic either.
	d.DecodeBlock([]byte{c0EXT1, c3DefineCustom, 0x00})
}

package cea708

import "testing"

func TestTruncatedP16AndEXT1(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)

	// P16 with only one trailing byte must not panic and must surface raw data.
	tokens, _ := d.DecodeBlock([]byte{c0P16, 0x4E})
	if len(tokens) != 1 || tokens[0].Kind != KindUnknown || len(tokens[0].Raw) != 2 {
		t.Fatalf("truncated P16 handled wrong: %+v", tokens)
	}

	// EXT1 with no following byte.
	tokens, _ = d.DecodeBlock([]byte{c0EXT1})
	if len(tokens) != 1 || tokens[0].Kind != KindUnknown || tokens[0].Raw[0] != c0EXT1 {
		t.Fatalf("dangling EXT1 handled wrong: %+v", tokens)
	}
}

func TestTruncatedC1Command(t *testing.T) {
	d := NewServiceDecoder(1)
	// ClearWindows expects one parameter byte; supply none.
	_, cmds := d.DecodeBlock([]byte{c1CLW})
	if len(cmds) != 1 || len(cmds[0].Raw) != 1 || cmds[0].Kind != 0 {
		t.Fatalf("truncated C1 not preserved: %+v", cmds)
	}
}

func TestGRLockingShift(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	// Lock GR to G3, then a GR byte resolves through G3; 0xA0 is the [CC] sign.
	tokens, _ := d.DecodeBlock([]byte{c0LS3GR, 0xA0})
	if tokens[0].Table != 3 || tokens[0].Kind != KindClosedCaption {
		t.Fatalf("GR lock to G3 failed: %+v", tokens[0])
	}

	// Lock GR to G2 and read an unmapped G2 index: must stay unknown.
	tokens, _ = d.DecodeBlock([]byte{c0LS2GR, 0xC0})
	if tokens[0].Kind != KindUnknown || tokens[0].Table != 2 {
		t.Fatalf("unmapped G2 via GR should be unknown: %+v", tokens[0])
	}
}

func TestUnknownG3Index(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	tokens, _ := d.DecodeBlock([]byte{c0EXT1, 0xB0})
	if tokens[0].Kind != KindUnknown || tokens[0].Table != 3 {
		t.Fatalf("unknown G3 index wrong: %+v", tokens[0])
	}
}

func TestC2SkipLengths(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	// EXT1 + C2 sub-code 0x0A (band 0x08..0x0F -> one extra byte), then 'A'.
	tokens, _ := d.DecodeBlock([]byte{c0EXT1, 0x0A, 0x77, 'A'})
	if len(tokens) != 2 {
		t.Fatalf("expected unknown C2 plus one char, got %d", len(tokens))
	}
	if tokens[0].Kind != KindUnknown || len(tokens[0].Raw) != 3 {
		t.Fatalf("C2 skip length wrong: %+v", tokens[0])
	}
	if tokens[1].Rune != 'A' {
		t.Fatalf("stream desynced after C2 skip: %+v", tokens[1])
	}
}

func TestGraphicWithoutWindow(t *testing.T) {
	d := NewServiceDecoder(1)
	// No window defined: tokens are still emitted, but nothing is written.
	tokens, _ := d.DecodeBlock([]byte{'X'})
	if len(tokens) != 1 || tokens[0].Rune != 'X' {
		t.Fatalf("token should still be emitted without a window: %+v", tokens)
	}
	if d.CurrentWindow() != nil {
		t.Fatal("no window should be selected")
	}
}

func TestCustomCharPixelBounds(t *testing.T) {
	c := CustomChar{Rows: 2, Cols: 3, Bitmap: []byte{0b11100000, 0b10100000}}
	if !c.Pixel(0, 2) || c.Pixel(0, 3) || c.Pixel(2, 0) {
		t.Fatal("pixel bounds check wrong")
	}
	if c.Pixel(1, 1) || !c.Pixel(1, 0) {
		t.Fatal("pixel bit decode wrong")
	}
}

func TestGraphicSetString(t *testing.T) {
	if G0.String() != "G0" || G3.String() != "G3" {
		t.Fatal("GraphicSet.String wrong")
	}
}

func TestSingleShiftDoesNotAffectGR(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	// A pending SS2 must not redirect a GR byte; 0xE9 still resolves via G1.
	tokens, _ := d.DecodeBlock([]byte{c0SS2, 0xE9})
	if tokens[0].Table != 1 || tokens[0].Rune != 'é' {
		t.Fatalf("SS2 leaked into GR region: %+v", tokens[0])
	}
	// The consumed single shift should have expired.
	if d.singleShift != -1 {
		t.Fatalf("single shift should be cleared, got %v", d.singleShift)
	}
}

func TestSingleShiftExpiresAtBlockBoundary(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	d.DecodeBlock([]byte{c0SS2}) // no graphic byte follows in this block
	if d.singleShift != -1 {
		t.Fatalf("dangling single shift must expire at block boundary, got %v", d.singleShift)
	}
	// Next block's GL byte must use the normal G0 designation.
	tokens, _ := d.DecodeBlock([]byte{'A'})
	if tokens[0].Table != 0 || tokens[0].Rune != 'A' {
		t.Fatalf("GL state corrupted after block boundary: %+v", tokens[0])
	}
}

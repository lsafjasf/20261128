package cea708

// This file holds the four graphic character sets defined by ANSI/CTA-708:
//
//	G0 : printable ASCII (0x20..0x7F), 0x7F is the musical note.
//	G1 : ISO/IEC 8859-1 Latin-1 (0xA0..0xFF).
//	G2 : extended miscellaneous symbols (indexed 0x20..0x7F).
//	G3 : the closed-caption sign and future extensions (indexed 0xA0..0xFF).
//
// A value of 0 means "not assigned in this revision of the standard"; callers
// must keep such code points observable instead of silently replacing them.

// g0Rune maps a G0 index (0x20..0x7F) to its rune.
func g0Rune(index byte) rune {
	switch {
	case index >= 0x20 && index <= 0x7E:
		return rune(index)
	case index == 0x7F:
		return '\u266A' // eighth note
	default:
		return 0
	}
}

// g1Rune maps a G1 index (0xA0..0xFF) to its rune. G1 is Latin-1 so the code
// points line up one-to-one with Unicode U+00A0..U+00FF.
func g1Rune(index byte) rune {
	if index >= 0xA0 {
		return rune(index)
	}
	return 0
}

// g2Rune maps a G2 index to its rune. Only the symbols enumerated by
// ANSI/CTA-708 (and SMPTE RP 2052-11 Table 13) are assigned. The two
// transparent-space codes (0x20/0x21) are not ordinary runes and are reported
// through dedicated token kinds; the lookup here returns 0 for them.
func g2Rune(index byte) rune {
	if r, ok := g2Table[index]; ok {
		return r
	}
	return 0
}

// g3Rune maps a G3 index to its rune. The current standard only defines the
// closed-caption sign at 0xA0, which has no single Unicode code point and is
// reported through a dedicated token kind; the lookup here returns 0.
func g3Rune(index byte) rune {
	return 0
}

var g2Table = map[byte]rune{
	0x25: '\u2026', // horizontal ellipsis
	0x2A: '\u0160', // Latin capital S with caron
	0x2C: '\u0152', // Latin capital ligature OE
	0x30: '\u2588', // full block
	0x31: '\u2018', // left single quotation mark
	0x32: '\u2019', // right single quotation mark
	0x33: '\u201C', // left double quotation mark
	0x34: '\u201D', // right double quotation mark
	0x35: '\u2022', // bullet
	0x39: '\u2122', // trade mark sign
	0x3A: '\u0161', // Latin small s with caron
	0x3C: '\u0153', // Latin small ligature oe
	0x3D: '\u2120', // service mark sign
	0x3F: '\u0178', // Latin capital Y with diaeresis
	0x76: '\u215B', // vulgar fraction one eighth
	0x77: '\u215C', // vulgar fraction three eighths
	0x78: '\u215D', // vulgar fraction five eighths
	0x79: '\u215E', // vulgar fraction seven eighths
	0x7A: '\u2502', // box drawings light vertical
	0x7B: '\u2510', // box drawings light down and left
	0x7C: '\u2514', // box drawings light up and right
	0x7D: '\u2500', // box drawings light horizontal
	0x7E: '\u2518', // box drawings light up and left
	0x7F: '\u250C', // box drawings light down and right
}

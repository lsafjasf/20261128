package cea708

// C0 control codes (CL region, 0x00..0x1F).
const (
	c0NUL  byte = 0x00
	c0ETX  byte = 0x03
	c0BS   byte = 0x08
	c0FF   byte = 0x0C
	c0CR   byte = 0x0D
	c0HCR  byte = 0x0E
	c0EXT1 byte = 0x10
	c0P16  byte = 0x18
)

// ISO/IEC 2022 style invocation codes. ANSI/CTA-708 keeps the GL/GR code-table
// machinery of ISO 2022 even though the default designations make it optional.
// The single-shift codes reach G2/G3 for one graphic byte; locking shifts and
// re-designations use the two-byte band handled in parse.go. 0x0F is free in
// the CEA-708 C0 table and acts as the GL->G0 locking shift (SI).
const (
	c0LS0 byte = 0x0F // locking shift GL -> G0 (SI)
	c0SS2 byte = 0x19 // single shift next graphic via G2
	c0SS3 byte = 0x1D // single shift next graphic via G3
)

// Two-byte table control codes occupy C0 positions 0x11..0x17. In addition to
// EXT1 (0x10), this band carries locking shifts and table designations; the
// first byte selects the operation and the second the target slot or page.
const (
	c0LS1GL byte = 0x11 // locking shift GL -> G1
	c0LS2GL byte = 0x12 // locking shift GL -> G2
	c0LS3GL byte = 0x13 // locking shift GL -> G3
	c0LS1GR byte = 0x14 // locking shift GR -> G1
	c0LS2GR byte = 0x15 // locking shift GR -> G2
	c0LS3GR byte = 0x16 // locking shift GR -> G3
	// 0x17 is the table designation (table select) operation.
	c0TableSelect byte = 0x17
)

// C1 command codes (CR region, 0x80..0x9F).
const (
	c1CW0 byte = 0x80
	c1CW2 byte = 0x82
	c1CW7 byte = 0x87
	c1CLW byte = 0x88
	c1DSW byte = 0x89
	c1HDW byte = 0x8A
	c1TGW byte = 0x8B
	c1DLW byte = 0x8C
	c1DLY byte = 0x8D
	c1DLC byte = 0x8E
	c1RST byte = 0x8F
	c1SPA byte = 0x90
	c1SPC byte = 0x91
	c1SPL byte = 0x92
	c1SWA byte = 0x97
	c1DF0 byte = 0x98
	c1DF3 byte = 0x9B
	c1DF7 byte = 0x9F
)

// Extended C3 code that installs a user-defined glyph, reached via EXT1.
const (
	c3DefineCustom byte = 0x90
)

// c1Length returns the total encoded length (command byte plus parameters) of
// a C1 command. Reserved/unknown commands are single byte.
func c1Length(code byte) int {
	switch {
	case code >= c1CW0 && code <= c1CW7:
		return 1
	case code >= c1CLW && code <= c1DLY:
		return 2
	case code == c1DLC || code == c1RST:
		return 1
	case code == c1SPA || code == c1SPL:
		return 3
	case code == c1SPC:
		return 4
	case code == c1SWA:
		return 5
	case code >= c1DF0 && code <= c1DF7:
		return 7
	default:
		return 1
	}
}

// c2ExtraLength reports how many additional bytes follow a C2 command reached
// through EXT1, based on its 0x00..0x1F sub-code bands.
func c2ExtraLength(sub byte) int {
	switch {
	case sub <= 0x07:
		return 0
	case sub <= 0x0F:
		return 1
	case sub <= 0x17:
		return 2
	default:
		return 3
	}
}

// c3ExtraLength reports additional bytes following an EXT1 C3 command.
func c3ExtraLength(sub byte) int {
	switch {
	case sub <= 0x87:
		return 4
	case sub <= 0x8F:
		return 5
	case sub == c3DefineCustom:
		// Header bytes plus a variable bitmap payload sized by the command.
		return 6
	default:
		if sub <= 0x9F {
			// 0x91..0x9F are variable-length future commands; without a known
			// length they consume no guaranteed extra bytes.
			return 0
		}
		return 0
	}
}

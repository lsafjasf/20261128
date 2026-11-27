package cea708

// Kind identifies the kind of a decoded Token.
type Kind int

const (
	// KindUnknown is emitted for every code point that has no assigned meaning
	// in the active table, or for an unsupported control/command code. The
	// original byte values are preserved verbatim in Raw.
	KindUnknown Kind = iota

	// KindChar is a printable graphic character taken from one of G0..G3.
	KindChar

	// KindTransparentSpace is the G2 TSP (0x20) symbol: a space cell that
	// must not be overwritten by background painting.
	KindTransparentSpace

	// KindNBTransparentSpace is the G2 NBTSP (0x21) symbol: a non-breaking
	// transparent space.
	KindNBTransparentSpace

	// KindClosedCaption is the G3 [CC] sign (G3 index 0xA0).
	KindClosedCaption

	// KindCustomChar references a glyph previously installed by a
	// DefineCustomChar command. The code point used to reference it is kept
	// in Raw and its bitmap is available from the service decoder.
	KindCustomChar

	// KindP16 is a 16-bit regional character introduced by the P16 C0 code.
	KindP16
)

// Token is a single decoded graphic element. Control and window-management
// commands are exposed through Command values rather than tokens.
type Token struct {
	// Kind classifies the token.
	Kind Kind

	// Table is the graphic set (0..3) the code point was interpreted with,
	// honoring locking and single shifts. It is -1 when not applicable
	// (KindP16 and KindUnknown control codes).
	Table int

	// Rune carries the Unicode value for KindChar. It is zero for the other
	// token kinds.
	Rune rune

	// Value is the 7-bit graphic index (for KindChar/KindCustomChar) or the
	// assembled 16-bit value for KindP16.
	Value int

	// Raw retains the exact source byte(s), including any shift prefix and
	// EXT1, so an unknown code can be reconstructed byte for byte.
	Raw []byte
}

// GraphicSet identifies one of the four designation slots.
type GraphicSet int

const (
	G0 GraphicSet = iota
	G1
	G2
	G3
)

func (g GraphicSet) String() string {
	switch g {
	case G0:
		return "G0"
	case G1:
		return "G1"
	case G2:
		return "G2"
	case G3:
		return "G3"
	default:
		return "G?"
	}
}

// TableID names a registered character table that may be designated into one
// of the G0..G3 slots. Only the tables defined by ANSI/CTA-708 are listed;
// additional regional pages are carried as numeric identifiers.
type TableID int

const (
	// TableDefaultASCII is the mandated G0 page (printable ASCII + note).
	TableDefaultASCII TableID = 0
	// TableLatin1 is the mandated G1 page (ISO 8859-1).
	TableLatin1 TableID = 1
	// TableExtendedMisc is the mandated G2 page.
	TableExtendedMisc TableID = 2
	// TableFuture is the mandated G3 page ([CC] sign).
	TableFuture TableID = 3
)

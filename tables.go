package cea608

// Mode is the current CEA-608 caption mode.
type Mode uint8

const (
	ModeCaption Mode = iota
	ModePaintOn
	ModeRollUp
	ModeText
)

const (
	displayed    = 0
	nonDisplayed = 1
)

var basicOverrides = map[byte]rune{
	0x27: '\u2019',
	0x2a: 'á',
	0x5c: 'é',
	0x5e: 'í',
	0x5f: 'ó',
	0x60: 'ú',
	0x7b: 'ç',
	0x7c: '÷',
	0x7d: 'Ñ',
	0x7e: 'ñ',
	0x7f: '█',
}

var specialCharacters = [...]rune{
	'®', '°', '½', '¿', '™', '¢', '£', '♪',
	'à', '\u00a0', 'è', 'â', 'ê', 'î', 'ô', 'û',
}

var extendedSpanishFrench = [...]rune{
	'Á', 'É', 'Ó', 'Ú', 'Ü', 'ü', '‘', '¡',
	'*', '\'', '—', '©', '℠', '•', '“', '”',
	'À', 'Â', 'Ç', 'È', 'Ê', 'Ë', 'ë', 'Î',
	'Ï', 'ï', 'Ô', 'Ù', 'ù', 'Û', '«', '»',
}

var extendedPortugueseGerman = [...]rune{
	'Ã', 'ã', 'Í', 'Ì', 'ì', 'Ò', 'ò', 'Õ',
	'õ', '{', '}', '\\', '^', '_', '|', '~',
	'Ä', 'ä', 'Ö', 'ö', 'ß', '¥', '¤', '¦',
	'Å', 'å', 'Ø', 'ø', '┌', '┐', '└', '┘',
}

var pacRows = [...]int{
	11, -1, 1, 2, 3, 4, 12, 13,
	14, 15, 5, 6, 7, 8, 9, 10,
}

var controlNames = map[byte]string{
	0x20: "RCL",
	0x21: "BS",
	0x22: "AOF",
	0x23: "AON",
	0x24: "DER",
	0x25: "RU2",
	0x26: "RU3",
	0x27: "RU4",
	0x28: "FON",
	0x29: "RDC",
	0x2a: "TR",
	0x2b: "RTD",
	0x2c: "EDM",
	0x2d: "CR",
	0x2e: "ENM",
	0x2f: "EOC",
}

// ControlCode values are stable identifiers. Control words use their
// canonical first byte (channel bit removed) and second byte. PACs retain
// their two transmitted data bytes.
const (
	ControlRCL ControlCode = 0x1420
	ControlBS  ControlCode = 0x1421
	ControlAOF ControlCode = 0x1422
	ControlAON ControlCode = 0x1423
	ControlDER ControlCode = 0x1424
	ControlRU2 ControlCode = 0x1425
	ControlRU3 ControlCode = 0x1426
	ControlRU4 ControlCode = 0x1427
	ControlFON ControlCode = 0x1428
	ControlRDC ControlCode = 0x1429
	ControlTR  ControlCode = 0x142a
	ControlRTD ControlCode = 0x142b
	ControlEDM ControlCode = 0x142c
	ControlCR  ControlCode = 0x142d
	ControlENM ControlCode = 0x142e
	ControlEOC ControlCode = 0x142f
)

func basicCharacter(b byte) rune {
	if r, ok := basicOverrides[b]; ok {
		return r
	}
	return rune(b)
}

func dataPair(first, second byte) ControlCode {
	return ControlCode(uint16(first)<<8 | uint16(second))
}

func canonicalControlFirst(first byte) byte {
	return first &^ 0x08
}

func channelOf(first byte) int {
	if first&0x08 != 0 {
		return 2
	}
	return 1
}

func isControlLead(b byte) bool {
	return b == 0x14 || b == 0x15 || b == 0x1c || b == 0x1d
}

func isSpecialLead(b byte) bool {
	return b == 0x11 || b == 0x19
}

func isExtendedLead(b byte) bool {
	return b == 0x12 || b == 0x13 || b == 0x1a || b == 0x1b
}

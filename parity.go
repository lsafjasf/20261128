package cea608

import "math/bits"

// ValidParity reports whether b has the odd parity required by CEA-608.
// Every transmitted byte, including padding bytes, must have an odd number
// of set bits.
func ValidParity(b byte) bool {
	return bits.OnesCount8(b)&1 == 1
}

// HasValidParity is an alias for ValidParity.
func HasValidParity(b byte) bool {
	return ValidParity(b)
}

// StripParity returns the seven data bits carried by b.
func StripParity(b byte) byte {
	return b & 0x7f
}

// AddOddParity returns b with an odd-parity bit added in bit 7. Only the low
// seven bits of b are used.
func AddOddParity(b byte) byte {
	b &= 0x7f
	if bits.OnesCount8(b)&1 == 0 {
		return b | 0x80
	}
	return b
}

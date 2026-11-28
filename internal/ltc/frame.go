package ltc

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const SyncBits = "0011111111111101"

type BitOrder string

const (
	BitOrderAuto BitOrder = "auto"
	BitOrderMSB  BitOrder = "msb"
	BitOrderLSB  BitOrder = "lsb"
)

func NormalizeBits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '0' || r == '1' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ParseBitString parses canonical bits with bit 0 first. An 80-bit frame must
// end in the LTC sync word; 64 information bits may omit it for ATC inputs.
func ParseBitString(s string, nominalFPS int) (Frame, error) {
	bits := NormalizeBits(s)
	if len(bits) != 64 && len(bits) != 80 {
		return Frame{}, fmt.Errorf("LTC frame has %d bits; want 64 or 80", len(bits))
	}
	return DecodeBits(bits, nominalFPS, len(bits) == 64), nil
}

func DecodeBits(bits string, nominalFPS int, infoOnly bool) Frame {
	f := Frame{Bits: bits, InfoBitsOnly: infoOnly}
	if len(bits) == 80 {
		f.SyncValid = bits[64:80] == SyncBits
		if !f.SyncValid {
			f.Warnings = append(f.Warnings, "sync word at bits 64..79 is not 0011111111111101")
		}
	}

	f.Timecode = Timecode{
		Hours:   bcd(bits, 48, 4) + 10*bcd(bits, 56, 2),
		Minutes: bcd(bits, 32, 4) + 10*bcd(bits, 40, 3),
		Seconds: bcd(bits, 16, 4) + 10*bcd(bits, 24, 3),
		Frames:  bcd(bits, 0, 4) + 10*bcd(bits, 8, 2),
	}
	f.DropFrame = bits[10] == '1'
	f.ColorFrame = bits[11] == '1'

	if nominalFPS == 25 {
		f.BGF = BinaryGroupFlags{BGF0: bits[27] == '1', BGF1: bits[58] == '1', BGF2: bits[43] == '1'}
		f.Polarity = bits[59] == '1'
	} else {
		f.BGF = BinaryGroupFlags{BGF0: bits[43] == '1', BGF1: bits[58] == '1', BGF2: bits[59] == '1'}
		f.Polarity = bits[27] == '1'
	}

	for i, pos := range []int{4, 12, 20, 28, 36, 44, 52, 60} {
		f.UserGroups[i] = uint8(bcd(bits, pos, 4))
	}
	f.UserBits = fmt.Sprintf("%X%X%X%X%X%X%X%X",
		f.UserGroups[0], f.UserGroups[1], f.UserGroups[2], f.UserGroups[3],
		f.UserGroups[4], f.UserGroups[5], f.UserGroups[6], f.UserGroups[7])

	validateFrame(&f, nominalFPS)
	f.Date = DecodeDate(f)
	return f
}

func bcd(bits string, pos, width int) int {
	v := 0
	for i := 0; i < width; i++ {
		if bits[pos+i] == '1' {
			v |= 1 << i
		}
	}
	return v
}

func validateFrame(f *Frame, nominalFPS int) {
	tc := f.Timecode
	if tc.Hours > 23 || tc.Minutes > 59 || tc.Seconds > 59 || tc.Frames >= nominalFPS {
		f.Warnings = append(f.Warnings, fmt.Sprintf("invalid time address %s for %d fps", tc, nominalFPS))
	}
	if f.DropFrame && nominalFPS != 30 {
		f.Warnings = append(f.Warnings, "drop-frame flag set on a non-30 fps stream")
	}
	if _, err := FrameNumber(tc, nominalFPS, f.DropFrame); err != nil {
		f.Warnings = append(f.Warnings, err.Error())
	}
}

// DecodeHexFrame decodes 20 hex digits (80 bits) or 16 hex digits (64
// information bits). Auto order is selected by sync-word validity.
func DecodeHexFrame(s string, nominalFPS int, order BitOrder) (Frame, error) {
	token := strings.TrimSpace(s)
	token = strings.TrimPrefix(token, "0x")
	token = strings.TrimPrefix(token, "0X")
	token = strings.NewReplacer(" ", "", "\t", "", "_", "", ":", "").Replace(token)
	raw, err := hex.DecodeString(token)
	if err != nil {
		return Frame{}, fmt.Errorf("invalid hex LTC frame %q: %w", s, err)
	}
	if len(raw) != 8 && len(raw) != 10 {
		return Frame{}, fmt.Errorf("hex LTC frame has %d bytes; want 8 or 10", len(raw))
	}
	return decodeBytes(raw, nominalFPS, order)
}

func decodeBytes(raw []byte, nominalFPS int, order BitOrder) (Frame, error) {
	switch order {
	case BitOrderMSB:
		bits := bytesToBits(raw, false)
		return DecodeBits(bits, nominalFPS, len(raw) == 8), nil
	case BitOrderLSB:
		bits := bytesToBits(raw, true)
		return DecodeBits(bits, nominalFPS, len(raw) == 8), nil
	case BitOrderAuto, "":
		msb := DecodeBits(bytesToBits(raw, false), nominalFPS, len(raw) == 8)
		lsb := DecodeBits(bytesToBits(raw, true), nominalFPS, len(raw) == 8)
		if frameScore(lsb) > frameScore(msb) {
			return lsb, nil
		}
		return msb, nil
	default:
		return Frame{}, fmt.Errorf("unknown bit order %q", order)
	}
}

func bytesToBits(raw []byte, lsbFirst bool) string {
	b := make([]byte, 0, len(raw)*8)
	for _, v := range raw {
		for i := 0; i < 8; i++ {
			bit := (v >> (7 - i)) & 1
			if lsbFirst {
				bit = (v >> i) & 1
			}
			if bit == 1 {
				b = append(b, '1')
			} else {
				b = append(b, '0')
			}
		}
	}
	return string(b)
}

func frameScore(f Frame) int {
	score := 0
	if f.SyncValid {
		score += 100
	}
	if len(f.Warnings) == 0 {
		score += 10
	}
	if f.Date.Status == DateDetermined {
		score += 5
	}
	return score
}

package ltc

import (
	"encoding/hex"
	"testing"
)

func testFrame(t *testing.T, tc Timecode, fps int, drop bool, groups [8]uint8,
	bgf0, bgf1, bgf2 bool) Frame {
	t.Helper()
	bits := []byte("0000000000000000000000000000000000000000000000000000000000000000" + SyncBits)
	setBCD(bits, 0, 4, tc.Frames%10)
	setBCD(bits, 8, 2, tc.Frames/10)
	setBCD(bits, 16, 4, tc.Seconds%10)
	setBCD(bits, 24, 3, tc.Seconds/10)
	setBCD(bits, 32, 4, tc.Minutes%10)
	setBCD(bits, 40, 3, tc.Minutes/10)
	setBCD(bits, 48, 4, tc.Hours%10)
	setBCD(bits, 56, 2, tc.Hours/10)
	setBit(bits, 10, drop)
	for i, pos := range []int{4, 12, 20, 28, 36, 44, 52, 60} {
		setBCD(bits, pos, 4, int(groups[i]))
	}
	if fps == 25 {
		setBit(bits, 27, bgf0)
		setBit(bits, 58, bgf1)
		setBit(bits, 43, bgf2)
	} else {
		setBit(bits, 43, bgf0)
		setBit(bits, 58, bgf1)
		setBit(bits, 59, bgf2)
	}
	f, err := ParseBitString(string(bits), fps)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func setBCD(bits []byte, pos, width, value int) {
	for i := 0; i < width; i++ {
		setBit(bits, pos+i, value&(1<<i) != 0)
	}
}

func setBit(bits []byte, pos int, value bool) {
	if value {
		bits[pos] = '1'
	} else {
		bits[pos] = '0'
	}
}

func setDateDDMMYY(groups *[8]uint8, day, month, year, tzCode int) {
	groups[0] = uint8(day % 10)
	groups[1] = uint8(day / 10)
	groups[2] = uint8(month % 10)
	groups[3] = uint8(month / 10)
	groups[4] = uint8(year % 10)
	groups[5] = uint8(year / 10)
	groups[6] = uint8(tzCode & 0x0F)
	groups[7] = uint8((tzCode >> 4) & 0x03)
}

func setDateMJD(groups *[8]uint8, mjd int) {
	for i := 0; i < 6; i++ {
		groups[i] = uint8(mjd % 10)
		mjd /= 10
	}
	groups[7] |= 0x08
}

func packBitsMSB(bits string) []byte {
	out := make([]byte, len(bits)/8)
	for i := range bits {
		if bits[i] == '1' {
			out[i/8] |= 1 << (7 - i%8)
		}
	}
	return out
}

func reverseBitsPerByte(in []byte) []byte {
	out := make([]byte, len(in))
	for i, b := range in {
		for j := 0; j < 8; j++ {
			out[i] |= ((b >> j) & 1) << (7 - j)
		}
	}
	return out
}

func frameHex(f Frame, reverse bool) string {
	raw := packBitsMSB(f.Bits)
	if reverse {
		raw = reverseBitsPerByte(raw)
	}
	return hex.EncodeToString(raw)
}

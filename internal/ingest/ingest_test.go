package ingest

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/lsafjasf/20261128/internal/ltc"
)

func ingestBits(tc ltc.Timecode, groups [8]uint8, bgf2 bool, rate ...int) string {
	bits := []byte("0000000000000000000000000000000000000000000000000000000000000000" + ltc.SyncBits)
	put := func(pos, width, value int) {
		for i := 0; i < width; i++ {
			if value&(1<<i) != 0 {
				bits[pos+i] = '1'
			}
		}
	}
	put(0, 4, tc.Frames%10)
	put(8, 2, tc.Frames/10)
	put(16, 4, tc.Seconds%10)
	put(24, 3, tc.Seconds/10)
	put(32, 4, tc.Minutes%10)
	put(40, 3, tc.Minutes/10)
	put(48, 4, tc.Hours%10)
	put(56, 2, tc.Hours/10)
	for i, pos := range []int{4, 12, 20, 28, 36, 44, 52, 60} {
		put(pos, 4, int(groups[i]))
	}
	if bgf2 {
		if len(rate) > 0 && rate[0] == 25 {
			bits[43] = '1'
		} else {
			bits[59] = '1'
		}
	}
	return string(bits)
}

func TestInfer25FPSFromST309BGF(t *testing.T) {
	groups := [8]uint8{7, 2, 1, 1, 6, 2, 8, 1}
	text := ingestBits(ltc.Timecode{Hours: 1, Minutes: 2, Seconds: 3, Frames: 4}, groups, true, 25)
	result, err := ParseBytes([]byte(text), Options{Format: FormatText})
	if err != nil {
		t.Fatal(err)
	}
	if result.NominalFPS != 25 || result.Frames[0].Date.Status != ltc.DateCenturyAmbiguous {
		t.Fatalf("result fps=%d date=%+v", result.NominalFPS, result.Frames[0].Date)
	}
}

func TestParseTextBitAndHexLines(t *testing.T) {
	a := ingestBits(ltc.Timecode{Hours: 1, Minutes: 2, Seconds: 3, Frames: 4}, [8]uint8{}, false)
	b := ingestBits(ltc.Timecode{Hours: 1, Minutes: 2, Seconds: 3, Frames: 5}, [8]uint8{}, false)
	text := "# bits first, hex second\n" + a + "\n" + hex.EncodeToString(mustPack(b)) + "\n"
	result, err := ParseBytes([]byte(text), Options{Format: FormatText})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Frames) != 2 || result.NominalFPS != 30 {
		t.Fatalf("result = %+v", result)
	}
	if result.Frames[0].Timecode.String() != "01:02:03:04" ||
		result.Frames[1].Timecode.String() != "01:02:03:05" {
		t.Fatalf("timecodes = %s, %s", result.Frames[0].Timecode, result.Frames[1].Timecode)
	}
}

func TestParsePackedBinaryBothBitOrders(t *testing.T) {
	bits := ingestBits(ltc.Timecode{Hours: 23, Minutes: 59, Seconds: 59, Frames: 29}, [8]uint8{}, false)
	packed := mustPack(bits)
	reversed := make([]byte, len(packed))
	for i, b := range packed {
		for j := 0; j < 8; j++ {
			reversed[i] |= ((b >> j) & 1) << (7 - j)
		}
	}
	for name, data := range map[string][]byte{"msb": packed, "lsb": reversed} {
		result, err := ParseBytes(data, Options{Format: FormatBinary})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(result.Frames) != 1 || result.Frames[0].Timecode.String() != "23:59:59:29" {
			t.Fatalf("%s: %+v", name, result.Frames)
		}
	}
}

func TestParseBitBytes(t *testing.T) {
	bits := ingestBits(ltc.Timecode{Hours: 0, Minutes: 0, Seconds: 0, Frames: 1}, [8]uint8{}, false)
	data := make([]byte, len(bits))
	for i := range bits {
		if bits[i] == '1' {
			data[i] = 1
		}
	}
	result, err := ParseBytes(data, Options{Format: FormatBinary})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Frames) != 1 || result.Frames[0].Timecode.String() != "00:00:00:01" {
		t.Fatalf("frames = %+v", result.Frames)
	}
}

func mustPack(bits string) []byte {
	out := make([]byte, len(bits)/8)
	for i := range bits {
		if bits[i] == '1' {
			out[i/8] |= 1 << (7 - i%8)
		}
	}
	return out
}

func TestTextContinuousBits(t *testing.T) {
	a := ingestBits(ltc.Timecode{Hours: 1, Minutes: 1, Seconds: 1, Frames: 1}, [8]uint8{}, false)
	b := ingestBits(ltc.Timecode{Hours: 1, Minutes: 1, Seconds: 1, Frames: 2}, [8]uint8{}, false)
	text := strings.Join([]string{a[:40], a[40:] + b}, "\n")
	result, err := ParseBytes([]byte(text), Options{Format: FormatText})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Frames) != 2 {
		t.Fatalf("frames = %d", len(result.Frames))
	}
}

func TestTextBitStreamAlignment(t *testing.T) {
	a := ingestBits(ltc.Timecode{Hours: 2, Minutes: 2, Seconds: 2, Frames: 2}, [8]uint8{}, false)
	b := ingestBits(ltc.Timecode{Hours: 2, Minutes: 2, Seconds: 2, Frames: 3}, [8]uint8{}, false)
	result, err := ParseBytes([]byte("10101"+a+b), Options{Format: FormatText})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Frames) != 2 || result.Frames[1].Timecode.String() != "02:02:02:03" {
		t.Fatalf("frames = %+v", result.Frames)
	}
}

func TestBinaryBitStreamAlignment(t *testing.T) {
	a := ingestBits(ltc.Timecode{Hours: 3, Minutes: 3, Seconds: 3, Frames: 3}, [8]uint8{}, false)
	b := ingestBits(ltc.Timecode{Hours: 3, Minutes: 3, Seconds: 3, Frames: 4}, [8]uint8{}, false)
	data := append([]byte{0xAA, 0x55, 0xF0}, mustPack(a+b)...)
	result, err := ParseBytes(data, Options{Format: FormatBinary})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Frames) != 2 || result.Frames[0].Timecode.String() != "03:03:03:03" {
		t.Fatalf("frames = %+v", result.Frames)
	}
}

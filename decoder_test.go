package cea608

import (
	"strings"
	"testing"
)

func pair(first, second byte) []byte {
	return []byte{AddOddParity(first), AddOddParity(second)}
}

func addPair(dst []byte, first, second byte) []byte {
	return append(dst, AddOddParity(first), AddOddParity(second))
}

func addRawPair(dst []byte, first, second byte) []byte {
	return append(dst, first, second)
}

func addText(dst []byte, text string) []byte {
	data := []byte(text)
	for i := 0; i < len(data); i += 2 {
		second := byte(0)
		if i+1 < len(data) {
			second = data[i+1]
		}
		dst = addPair(dst, data[i], second)
	}
	return dst
}

func sequence(caption string) []byte {
	var data []byte
	data = addPair(data, 0x14, 0x20)
	data = addPair(data, 0x14, 0x20)
	data = addText(data, caption)
	data = addPair(data, 0x14, 0x2f)
	data = addPair(data, 0x14, 0x2f)
	return data
}

func textEvents(events []Event) string {
	var text strings.Builder
	for _, event := range events {
		if event.Type == EventText {
			text.WriteString(event.Text)
		}
	}
	return text.String()
}

func countEvents(events []Event, kind EventType) int {
	count := 0
	for _, event := range events {
		if event.Type == kind {
			count++
		}
	}
	return count
}

func captions(events []Event) []string {
	var result []string
	for _, event := range events {
		if event.Type == EventCaption {
			result = append(result, event.Caption.Text)
		}
	}
	return result
}

func flipBit(b byte, bit uint) byte {
	return b ^ (1 << bit)
}

func TestOddParityHelpers(t *testing.T) {
	for value := 0; value < 0x80; value++ {
		encoded := AddOddParity(byte(value))
		if !ValidParity(encoded) {
			t.Fatalf("value %02X encodes to %02X with even parity", value, encoded)
		}
		if got := StripParity(encoded); got != byte(value) {
			t.Fatalf("strip(%02X) = %02X, want %02X", encoded, got, value)
		}
		for bit := uint(0); bit < 8; bit++ {
			if ValidParity(flipBit(encoded, bit)) {
				t.Fatalf("%02X remains valid after flipping bit %d", encoded, bit)
			}
		}
	}
	if ValidParity(0) {
		t.Fatal("zero byte must not have odd parity")
	}
	if !ValidParity(0x80) {
		t.Fatal("0x80 is legal CEA-608 padding")
	}
}

func TestDecodesAndReplacesDuplicateEOC(t *testing.T) {
	d := NewDecoder()
	events := d.Feed(sequence("Hi"))

	if got := textEvents(events); got != "Hi" {
		t.Fatalf("text events = %q, want %q", got, "Hi")
	}
	if got := captions(events); len(got) != 1 || got[0] != "Hi" {
		t.Fatalf("captions = %#v, want one Hi caption", got)
	}
	if got := d.Displayed(); got != "Hi" {
		t.Fatalf("displayed = %q, want Hi", got)
	}
	if got := countEvents(events, EventError); got != 0 {
		t.Fatalf("unexpected errors: %d", got)
	}
}

func TestConsecutiveCaptionsUseHiddenMemory(t *testing.T) {
	data := sequence("LONG CAPTION")
	data = append(data, sequence("NEW")...)

	d := NewDecoder()
	events := d.Feed(data)
	got := captions(events)
	if len(got) != 2 || got[0] != "LONG CAPTION" || got[1] != "NEW" {
		t.Fatalf("captions = %#v, want [LONG CAPTION NEW]", got)
	}
	if got := d.Displayed(); got != "NEW" {
		t.Fatalf("displayed = %q, want NEW", got)
	}
}

func TestKeepsLegalFirstByteWhenPartnerHasBadParity(t *testing.T) {
	var data []byte
	data = addPair(data, 0x14, 0x20)
	data = addRawPair(data, AddOddParity('A'), flipBit(AddOddParity('B'), 2))
	data = addText(data, "CD")
	data = addPair(data, 0x14, 0x2f)

	d := NewDecoder()
	events := d.Feed(data)
	if got := textEvents(events); got != "ACD" {
		t.Fatalf("text events = %q, want ACD", got)
	}
	if got := countEvents(events, EventError); got != 1 {
		t.Fatalf("error events = %d, want 1", got)
	}
	if got := d.Displayed(); got != "ACD" {
		t.Fatalf("displayed = %q, want ACD", got)
	}
}

func TestBadControlFirstByteDoesNotLeakSecondByte(t *testing.T) {
	var data []byte
	data = addPair(data, 0x14, 0x20)
	data = addText(data, "HI")
	data = addRawPair(data, flipBit(AddOddParity(0x14), 0), AddOddParity(0x2f))

	data = addPair(data, 0x14, 0x2e)
	data = addPair(data, 0x14, 0x20)
	data = addText(data, "OK")
	data = addPair(data, 0x14, 0x2f)

	d := NewDecoder()
	events := d.Feed(data)
	if got := textEvents(events); got != "HIOK" {
		t.Fatalf("text events = %q, want HIOK", got)
	}
	if got := captions(events); len(got) != 1 || got[0] != "OK" {
		t.Fatalf("captions = %#v, want only OK", got)
	}
	if got := countEvents(events, EventError); got != 1 {
		t.Fatalf("error events = %d, want 1", got)
	}
	if got := d.Displayed(); got != "OK" {
		t.Fatalf("displayed = %q, want OK", got)
	}
}

func TestBadControlSecondByteDoesNotBecomeText(t *testing.T) {
	var data []byte
	data = addPair(data, 0x14, 0x20)
	data = addText(data, "HI")
	data = addRawPair(data, AddOddParity(0x14), flipBit(AddOddParity(0x2f), 1))

	data = addPair(data, 0x14, 0x2e)
	data = addPair(data, 0x14, 0x20)
	data = addText(data, "OK")
	data = addPair(data, 0x14, 0x2f)

	d := NewDecoder()
	events := d.Feed(data)
	if got := textEvents(events); got != "HIOK" {
		t.Fatalf("text events = %q, want HIOK", got)
	}
	if got := captions(events); len(got) != 1 || got[0] != "OK" {
		t.Fatalf("captions = %#v, want only OK", got)
	}
	if got := d.Displayed(); got != "OK" {
		t.Fatalf("displayed = %q, want OK", got)
	}
}

func TestPairBoundarySurvivesChunkedFeed(t *testing.T) {
	var data []byte
	data = addPair(data, 0x14, 0x20)
	data = addText(data, "A")
	data = addRawPair(data, flipBit(AddOddParity(0x14), 4), AddOddParity(0x2f))
	data = addPair(data, 0x14, 0x2e)
	data = addPair(data, 0x14, 0x20)
	data = addText(data, "BC")
	data = addPair(data, 0x14, 0x2f)

	d := NewDecoder()
	var events []Event
	for _, b := range data {
		events = append(events, d.Feed([]byte{b})...)
	}

	if got := captions(events); len(got) != 1 || got[0] != "BC" {
		t.Fatalf("captions = %#v, want only BC", got)
	}
	if got := countEvents(events, EventError); got != 1 {
		t.Fatalf("error events = %d, want 1", got)
	}
	if got := d.Displayed(); got != "BC" {
		t.Fatalf("displayed = %q, want BC", got)
	}
}

func TestEverySingleBitFlipOfControlPairIsRecoverable(t *testing.T) {
	for byteIndex := 0; byteIndex < 2; byteIndex++ {
		for bit := uint(0); bit < 8; bit++ {
			t.Run(string(rune('0'+byteIndex))+string(rune('0'+bit)), func(t *testing.T) {
				control := pair(0x14, 0x2f)
				control[byteIndex] = flipBit(control[byteIndex], bit)

				var data []byte
				data = addPair(data, 0x14, 0x20)
				data = addText(data, "BAD")
				data = append(data, control...)
				data = addPair(data, 0x14, 0x2e)
				data = addPair(data, 0x14, 0x20)
				data = addText(data, "NEXT")
				data = addPair(data, 0x14, 0x2f)

				d := NewDecoder()
				events := d.Feed(data)
				if got := captions(events); len(got) != 1 || got[0] != "NEXT" {
					t.Fatalf("captions = %#v, want only NEXT", got)
				}
				if got := d.Displayed(); got != "NEXT" {
					t.Fatalf("displayed = %q, want NEXT", got)
				}
				if got := countEvents(events, EventError); got != 1 {
					t.Fatalf("error events = %d, want 1", got)
				}
			})
		}
	}
}

func TestMalformedParityValidControlIsRejected(t *testing.T) {
	var data []byte
	data = addPair(data, 0x14, 0x20)
	data = addText(data, "A")
	data = addPair(data, 0x14, 0x30)
	data = addText(data, "B")
	data = addPair(data, 0x14, 0x2f)

	d := NewDecoder()
	events := d.Feed(data)
	if got := captions(events); len(got) != 1 || got[0] != "AB" {
		t.Fatalf("captions = %#v, want AB", got)
	}
	if got := countEvents(events, EventError); got != 1 {
		t.Fatalf("error events = %d, want 1", got)
	}
	for _, event := range events {
		if event.Type == EventError && event.Error.Kind != ErrorMalformedPair {
			t.Fatalf("error kind = %v, want malformed", event.Error.Kind)
		}
	}
}

func TestPACRowsIndentAndSpecialCharacters(t *testing.T) {
	var data []byte
	data = addPair(data, 0x14, 0x20)
	data = addPair(data, 0x11, 0x40)
	data = addText(data, "A")
	data = addPair(data, 0x12, 0x52)
	data = addText(data, "B")
	data = addPair(data, 0x11, 0x37)
	data = addPair(data, 0x12, 0x20)
	data = addPair(data, 0x14, 0x2f)

	d := NewDecoder()
	events := d.Feed(data)
	if got := captions(events); len(got) != 1 {
		t.Fatalf("captions = %#v, want one caption", got)
	}
	caption := events[len(events)-1].Caption
	want := "A\n\n    B♪Á"
	if caption.Text != want {
		t.Fatalf("caption = %q, want %q", caption.Text, want)
	}
	if got := d.Displayed(); got != want {
		t.Fatalf("displayed = %q, want %q", got, want)
	}
}

func TestResetReestablishesPairBoundary(t *testing.T) {
	d := NewDecoder()
	d.Feed([]byte{AddOddParity('A')})
	d.Reset()
	events := d.Feed(sequence("OK"))
	if got := d.Displayed(); got != "OK" {
		t.Fatalf("displayed = %q, want OK", got)
	}
	if got := countEvents(events, EventError); got != 0 {
		t.Fatalf("errors after reset = %d", got)
	}
}

func TestZeroValueDecoderIsUsable(t *testing.T) {
	var d Decoder
	events := d.Feed(sequence("ZERO"))
	if got := d.Displayed(); got != "ZERO" {
		t.Fatalf("displayed = %q, want ZERO", got)
	}
	if got := countEvents(events, EventError); got != 0 {
		t.Fatalf("errors = %d, want 0", got)
	}
}

func TestChannelTwoControlWordsUseSameCommands(t *testing.T) {
	var data []byte
	data = addPair(data, 0x1c, 0x20)
	data = addText(data, "C2")
	data = addPair(data, 0x1c, 0x2f)

	d := NewDecoder()
	events := d.Feed(data)
	if got := d.Displayed(); got != "C2" {
		t.Fatalf("displayed = %q, want C2", got)
	}
	foundChannelTwo := false
	for _, event := range events {
		if event.Type == EventControl && event.Control.First == 0x1c {
			foundChannelTwo = true
			if event.Control.Channel != 2 {
				t.Fatalf("channel = %d, want 2", event.Control.Channel)
			}
			if event.Control.Name != "RCL" && event.Control.Name != "EOC" {
				t.Fatalf("unexpected command %q", event.Control.Name)
			}
		}
	}
	if !foundChannelTwo {
		t.Fatal("channel 2 control event not found")
	}
}

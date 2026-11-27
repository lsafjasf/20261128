package cea608

// Decoder converts a stream of CEA-608 bytes into caption events.
//
// Bytes are consumed in transmission order and always remain paired across
// Feed calls. A rejected pair cannot shift the alignment of later pairs.
// Decode errors are returned as EventError values so that decoding can
// continue with the next caption.
type Decoder struct {
	firstRaw    byte
	firstOffset int64
	haveFirst   bool

	offset int64
	stats  Stats

	dupValid bool
	dupCode  ControlCode

	memory       [2][15][32]rune
	target       int
	cursorRow    int
	cursorColumn int
	mode         Mode
	rollUpRows   int
	displayed    int
	initialized  bool
}

// NewDecoder returns a decoder with caption memory initialized to row 1.
func NewDecoder() *Decoder {
	d := &Decoder{}
	d.Reset()
	return d
}

// Reset clears all decoding state and accumulated statistics.
func (d *Decoder) Reset() {
	*d = Decoder{}
	d.target = nonDisplayed
	d.displayed = displayed
	d.cursorRow = 0
	d.cursorColumn = 0
	d.mode = ModeCaption
	d.clearMemory(displayed)
	d.clearMemory(nonDisplayed)
	d.initialized = true
}

// Feed consumes bytes and returns all events completed by this call. A final
// odd byte is retained until the next call so pair boundaries never move.
func (d *Decoder) Feed(p []byte) []Event {
	if !d.initialized {
		d.Reset()
	}
	var events []Event
	for _, b := range p {
		d.stats.Bytes++
		if !d.haveFirst {
			d.firstRaw = b
			d.firstOffset = d.offset
			d.haveFirst = true
			d.offset++
			continue
		}

		first := d.firstRaw
		offset := d.firstOffset
		d.haveFirst = false
		d.offset++
		d.stats.Pairs++

		events = append(events, d.consumePair(first, b, offset)...)
	}
	return events
}

// Write implements io.Writer. It decodes bytes exactly as Feed does, but
// discards the returned events; use Feed when event data is needed.
func (d *Decoder) Write(p []byte) (int, error) {
	d.Feed(p)
	return len(p), nil
}

// Stats returns a snapshot of cumulative decoder statistics.
func (d *Decoder) Stats() Stats {
	return d.stats
}

// Decode decodes one complete stream and returns its events.
func Decode(p []byte) []Event {
	return NewDecoder().Feed(p)
}

func (d *Decoder) consumePair(first, second byte, offset int64) []Event {
	firstOK := ValidParity(first)
	secondOK := ValidParity(second)

	if !firstOK {
		if !secondOK {
			d.stats.ParityErrors += 2
		} else {
			d.stats.ParityErrors++
		}
		return d.rejectPair(first, second, offset, 0, true)
	}
	if !secondOK {
		d.stats.ParityErrors++
		data := StripParity(first)
		if isBasicCharacter(data) {
			// The first half of the pair is independently legal. Keep it,
			// but never let the failed second byte become text.
			d.resetDuplicate()
			d.writeCharacter(basicCharacter(data))
			return []Event{
				{Type: EventText, Text: string(basicCharacter(data))},
				d.errorEvent(first, second, offset, 1, ErrorParity),
			}
		}
		return d.rejectPair(first, second, offset, 1, true)
	}

	return d.consumeValidPair(first, second, offset)
}

func (d *Decoder) consumeValidPair(first, second byte, offset int64) []Event {
	a := StripParity(first)
	b := StripParity(second)

	if a == 0 {
		if b == 0 {
			d.stats.ValidPairs++
			d.resetDuplicate()
			return nil
		}
		return d.rejectPair(first, second, offset, 0, false)
	}

	if isBasicCharacter(a) {
		d.resetDuplicate()
		text := string(basicCharacter(a))
		if isBasicCharacter(b) {
			text += string(basicCharacter(b))
			d.stats.ValidPairs++
			d.writeText(text)
			return []Event{{Type: EventText, Text: text}}
		}
		d.stats.ValidPairs++
		d.writeText(text)
		events := []Event{{Type: EventText, Text: text}}
		if b != 0 {
			d.stats.Malformed++
			events = append(events, d.errorEvent(first, second, offset, 1, ErrorMalformedPair))
		}
		return events
	}

	if c, ok := decodeControl(a, b); ok {
		d.stats.ValidPairs++
		return d.acceptControl(c)
	}
	if text, ok := decodeSpecial(a, b); ok {
		d.stats.ValidPairs++
		d.resetDuplicate()
		d.writeText(text)
		return []Event{{Type: EventText, Text: text}}
	}
	if text, ok := decodeExtended(a, b); ok {
		d.stats.ValidPairs++
		d.resetDuplicate()
		d.writeText(text)
		return []Event{{Type: EventText, Text: text}}
	}

	return d.rejectPair(first, second, offset, 0, false)
}

func (d *Decoder) rejectPair(first, second byte, offset int64, byteIndex int, parity bool) []Event {
	kind := ErrorMalformedPair
	if parity {
		kind = ErrorParity
	} else {
		d.stats.Malformed++
	}
	d.stats.DroppedPairs++
	d.resetDuplicate()
	return []Event{d.errorEvent(first, second, offset, byteIndex, kind)}
}

func (d *Decoder) errorEvent(first, second byte, offset int64, byteIndex int, kind ErrorKind) Event {
	return Event{
		Type: EventError,
		Error: &DecodeError{
			Kind:      kind,
			Pair:      [2]byte{first, second},
			ByteIndex: byteIndex,
			Offset:    offset,
		},
	}
}

func (d *Decoder) resetDuplicate() {
	d.dupValid = false
	d.dupCode = 0
}

func isBasicCharacter(b byte) bool {
	return b >= 0x20 && b <= 0x7f
}

package cea708

// DecodeBlock feeds one service block's caption byte stream into the decoder.
// Window state, table invocation state and downloaded glyphs are updated in
// place. The decoded Tokens and Commands are also returned (and buffered on the
// ServiceDecoder) in stream order. Unknown code points are emitted with Kind
// KindUnknown and the original bytes preserved rather than being rendered.
func (d *ServiceDecoder) DecodeBlock(data []byte) ([]Token, []Command) {
	d.Tokens = d.Tokens[:0]
	d.Commands = d.Commands[:0]

	for i := 0; i < len(data); {
		b := data[i]
		switch {
		case b <= 0x1F:
			i = d.handleC0(data, i)
		case b <= 0x7F:
			i = d.handleGraphic(data, i, true)
		case b <= 0x9F:
			i = d.handleC1(data, i)
		default:
			i = d.handleGraphic(data, i, false)
		}
		if i <= 0 {
			// Defensive: a handler must always make forward progress.
			i++
		}
	}

	// A single shift that was never followed by a graphic byte expires at the
	// block boundary.
	d.singleShift = -1
	return d.Tokens, d.Commands
}

func (d *ServiceDecoder) emitToken(t Token) {
	d.Tokens = append(d.Tokens, t)
	if w := d.CurrentWindow(); w != nil {
		if t.Kind != KindUnknown {
			w.write(t)
		}
	}
}

func (d *ServiceDecoder) emitCommand(c Command) {
	d.Commands = append(d.Commands, c)
}

// handleGraphic decodes one GL (left=true, index 0x20..0x7F) or GR
// (left=false, index 0xA0..0xFF) graphic byte.
func (d *ServiceDecoder) handleGraphic(data []byte, i int, left bool) int {
	index := data[i]
	raw := []byte{index}

	slot := d.gl
	if !left {
		slot = d.gr
	}
	// A single shift only invokes G2/G3 into the GL region for one byte; it
	// does not alter the GR invocation, so GR bytes ignore a pending SS2/SS3.
	if left && d.singleShift >= 0 {
		slot = d.singleShift
		d.singleShift = -1
	}

	tok := d.decodeGraphic(slot, index, raw)
	d.emitToken(tok)
	return i + 1
}

// decodeGraphic resolves one graphic index in the given designation slot,
// consulting any downloaded glyph before the built-in table.
func (d *ServiceDecoder) decodeGraphic(slot GraphicSet, index byte, raw []byte) Token {
	if glyph, ok := d.custom[customKey{slot, index}]; ok {
		_ = glyph
		return Token{
			Kind:  KindCustomChar,
			Table: int(slot),
			Value: int(index),
			Raw:   append([]byte(nil), raw...),
		}
	}

	var r rune
	switch d.designation[slot] {
	case TableDefaultASCII:
		r = g0Rune(index)
	case TableLatin1:
		r = g1Rune(index)
	case TableExtendedMisc:
		r = g2Rune(index)
		// G2 transparent spaces and any unmapped index need special handling.
		if r == 0 {
			return d.decodeSpecialG2(slot, index, raw)
		}
	case TableFuture:
		if index == 0xA0 {
			return Token{Kind: KindClosedCaption, Table: int(slot), Value: int(index), Raw: append([]byte(nil), raw...)}
		}
		return Token{Kind: KindUnknown, Table: int(slot), Value: int(index), Raw: append([]byte(nil), raw...)}
	default:
		// Unknown regional page: keep the index observable.
		return Token{Kind: KindUnknown, Table: int(slot), Value: int(index), Raw: append([]byte(nil), raw...)}
	}

	if r == 0 {
		return Token{Kind: KindUnknown, Table: int(slot), Value: int(index), Raw: append([]byte(nil), raw...)}
	}
	return Token{Kind: KindChar, Table: int(slot), Rune: r, Value: int(index), Raw: append([]byte(nil), raw...)}
}

func (d *ServiceDecoder) decodeSpecialG2(slot GraphicSet, index byte, raw []byte) Token {
	switch index {
	case 0x20:
		return Token{Kind: KindTransparentSpace, Table: int(slot), Value: int(index), Raw: append([]byte(nil), raw...)}
	case 0x21:
		return Token{Kind: KindNBTransparentSpace, Table: int(slot), Value: int(index), Raw: append([]byte(nil), raw...)}
	default:
		return Token{Kind: KindUnknown, Table: int(slot), Value: int(index), Raw: append([]byte(nil), raw...)}
	}
}

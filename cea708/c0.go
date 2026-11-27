package cea708

// handleC0 consumes one C0 control code starting at data[i] and returns the
// index just past it.
func (d *ServiceDecoder) handleC0(data []byte, i int) int {
	code := data[i]
	raw := []byte{code}

	// Single-byte editing controls.
	switch code {
	case c0NUL, c0ETX:
		return i + 1
	case c0BS:
		if w := d.CurrentWindow(); w != nil {
			w.backspace()
		}
		return i + 1
	case c0FF:
		if w := d.CurrentWindow(); w != nil {
			w.clear()
		}
		return i + 1
	case c0CR:
		if w := d.CurrentWindow(); w != nil {
			w.carriageReturn()
		}
		return i + 1
	case c0HCR:
		if w := d.CurrentWindow(); w != nil {
			w.horizontalCarriageReturn()
		}
		return i + 1
	case c0LS0:
		d.gl = G0
		return i + 1
	case c0LS1GL:
		d.gl = G1
		return i + 1
	case c0LS2GL:
		d.gl = G2
		return i + 1
	case c0LS3GL:
		d.gl = G3
		return i + 1
	case c0LS1GR:
		d.gr = G1
		return i + 1
	case c0LS2GR:
		d.gr = G2
		return i + 1
	case c0LS3GR:
		d.gr = G3
		return i + 1
	case c0SS2:
		d.singleShift = G2
		return i + 1
	case c0SS3:
		d.singleShift = G3
		return i + 1
	case c0EXT1:
		return d.handleEXT1(data, i)
	case c0P16:
		return d.handleP16(data, i)
	}

	// Two-byte table select band (0x17). The locking-shift codes 0x11..0x16 are
	// single byte and handled above; only table select consumes a second byte.
	if code == c0TableSelect {
		if i+1 >= len(data) {
			d.emitToken(Token{Kind: KindUnknown, Table: -1, Raw: raw})
			return len(data)
		}
		second := data[i+1]
		two := []byte{code, second}
		d.handleTableControl(code, second, two)
		return i + 2
	}

	// Any other C0 code (0x01,0x02,0x04..0x07,0x09..0x0B,0x1A..0x1F besides the
	// named ones) is reserved: keep its raw value but do not render it.
	d.emitToken(Token{Kind: KindUnknown, Table: -1, Value: int(code), Raw: raw})
	return i + 1
}

// handleTableControl applies the two-byte locking-shift and table-select
// operations. The second byte identifies the target slot (locking shifts) or
// the slot/page pair (table select).
func (d *ServiceDecoder) handleTableControl(first, second byte, raw []byte) {
	switch first {
	case c0TableSelect:
		slot := GraphicSet(second >> 6)
		page := TableID(second & 0x3F)
		if slot <= G3 {
			d.designation[slot] = page
		} else {
			d.emitToken(Token{Kind: KindUnknown, Table: -1, Value: int(first), Raw: append([]byte(nil), raw...)})
		}
	}
}

// handleP16 consumes the P16 introducer and its two regional character bytes.
func (d *ServiceDecoder) handleP16(data []byte, i int) int {
	if i+2 >= len(data) {
		// Truncated: surface the bytes we do have and stop cleanly.
		d.emitToken(Token{Kind: KindUnknown, Table: -1, Raw: append([]byte(nil), data[i:]...)})
		return len(data)
	}
	b1, b2 := data[i+1], data[i+2]
	value := int(b1)<<8 | int(b2)
	raw := []byte{c0P16, b1, b2}
	d.emitToken(Token{Kind: KindP16, Table: -1, Value: value, Raw: raw})
	return i + 3
}

// handleEXT1 dispatches the EXT1 (0x10) introducer. EXT1 reaches the C2, G2,
// C3 and G3 sets for the following byte.
func (d *ServiceDecoder) handleEXT1(data []byte, i int) int {
	if i+1 >= len(data) {
		d.emitToken(Token{Kind: KindUnknown, Table: -1, Raw: []byte{c0EXT1}})
		return len(data)
	}
	sub := data[i+1]
	switch {
	case sub <= 0x1F:
		return d.handleC2(data, i)
	case sub <= 0x7F:
		// G2 graphic reached directly via EXT1, independent of invocation.
		raw := []byte{c0EXT1, sub}
		tok := d.decodeGraphic(G2, sub, raw)
		d.emitToken(tok)
		return i + 2
	case sub <= 0x9F:
		return d.handleC3(data, i)
	default:
		// G3 graphic reached directly via EXT1.
		raw := []byte{c0EXT1, sub}
		tok := d.decodeGraphic(G3, sub, raw)
		d.emitToken(tok)
		return i + 2
	}
}

// handleC2 skips a not-yet-defined C2 command while consuming its full length.
func (d *ServiceDecoder) handleC2(data []byte, i int) int {
	sub := data[i+1]
	extra := c2ExtraLength(sub)
	end := i + 2 + extra
	if end > len(data) {
		end = len(data)
	}
	d.emitToken(Token{Kind: KindUnknown, Table: -1, Value: int(sub), Raw: append([]byte(nil), data[i:end]...)})
	return end
}

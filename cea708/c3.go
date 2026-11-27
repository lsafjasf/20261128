package cea708

// handleC3 decodes an EXT1-introduced C3 command. The only command with a
// defined effect here is the user-defined glyph download (sub-code 0x90);
// other C3 commands are skipped over the number of bytes mandated by the
// standard while remaining observable through their raw bytes.
func (d *ServiceDecoder) handleC3(data []byte, i int) int {
	sub := data[i+1]

	if sub == c3DefineCustom {
		return d.handleDefineCustom(data, i)
	}

	extra := c3ExtraLength(sub)
	end := i + 2 + extra
	if end > len(data) {
		end = len(data)
	}
	d.emitCommand(Command{Raw: append([]byte(nil), data[i:end]...)})
	return end
}

// handleDefineCustom parses a DefineCustomChar sequence laid out as:
//
//	EXT1(0x10) 0x90            introducer + C3 sub-code
//	b0 (i+2)                   target slot (high nibble) + flags
//	b1 (i+3)                   graphic index the glyph is referenced by
//	b2 (i+4)                   rows (high nibble) / cols (low nibble)
//	i+5..                      packed bitmap, row-major, MSB-first
//
// The bitmap length is derived from rows*ceil(cols/8). A downloaded glyph
// lives until the (slot, index) pair is redefined or a Reset restores
// power-on state. Glyphs are service-scoped and may be shared by windows, so
// deleting one window does not remove them.
func (d *ServiceDecoder) handleDefineCustom(data []byte, i int) int {
	// i points at EXT1, i+1 at 0x90, then b0/b1/b2. We need five bytes total
	// before the bitmap begins.
	if i+5 > len(data) {
		end := len(data)
		d.emitCommand(Command{Kind: CmdDefineCustomChar, Raw: append([]byte(nil), data[i:end]...)})
		return end
	}

	b0 := data[i+2]
	index := data[i+3]
	b2 := data[i+4]

	set := GraphicSet((b0 >> 4) & 0x03)
	rows := int((b2>>4)&0x0F) + 1
	cols := int(b2&0x0F) + 1
	stride := (cols + 7) / 8
	bitmapLen := rows * stride

	end := i + 5 + bitmapLen
	if end > len(data) {
		// Truncated download: keep what arrived but do not install a partial
		// glyph that references missing rows.
		d.emitCommand(Command{Kind: CmdDefineCustomChar, Raw: append([]byte(nil), data[i:]...)})
		return len(data)
	}

	bitmap := append([]byte(nil), data[i+5:end]...)
	glyph := CustomChar{Set: set, Index: index, Rows: rows, Cols: cols, Bitmap: bitmap}
	d.custom[customKey{set, index}] = glyph

	d.emitCommand(Command{Kind: CmdDefineCustomChar, Custom: glyph, Raw: append([]byte(nil), data[i:end]...)})
	return end
}

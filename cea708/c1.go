package cea708

// handleC1 decodes a C1 caption command at data[i] and applies its effect.
func (d *ServiceDecoder) handleC1(data []byte, i int) int {
	code := data[i]
	length := c1Length(code)
	end := i + length
	if end > len(data) {
		// Truncated command: preserve the remainder as an unknown code.
		d.emitCommand(Command{Raw: append([]byte(nil), data[i:]...)})
		return len(data)
	}
	raw := append([]byte(nil), data[i:end]...)

	switch {
	case code >= c1CW0 && code <= c1CW7:
		window := int(code - c1CW0)
		if d.windows[window].Defined {
			d.current = window
		}
		d.emitCommand(Command{Kind: CmdSetCurrentWindow, Window: window, Raw: raw})

	case code >= c1CLW && code <= c1DLW:
		bitmap := data[i+1]
		kind := map[byte]CmdKind{c1CLW: CmdClearWindows, c1DSW: CmdDisplayWindows,
			c1HDW: CmdHideWindows, c1TGW: CmdToggleWindows, c1DLW: CmdDeleteWindows}[code]
		d.applyWindowGroup(kind, bitmap)
		d.emitCommand(Command{Kind: kind, Bitmap: bitmap, Raw: raw})

	case code == c1DLY:
		tenths := int(data[i+1] & 0x7F)
		d.emitCommand(Command{Kind: CmdDelay, DelayTenths: tenths, Raw: raw})

	case code == c1DLC:
		d.emitCommand(Command{Kind: CmdDelayCancel, Raw: raw})

	case code == c1RST:
		d.Reset()
		d.emitCommand(Command{Kind: CmdReset, Raw: raw})

	case code == c1SPA:
		attr := decodePenAttributes(data[i+1], data[i+2])
		d.emitCommand(Command{Kind: CmdSetPenAttributes, PenAttr: attr, Raw: raw})

	case code == c1SPC:
		color := decodePenColor(data[i+1], data[i+2], data[i+3])
		d.emitCommand(Command{Kind: CmdSetPenColor, PenColor: color, Raw: raw})

	case code == c1SPL:
		loc := PenLocation{Row: int(data[i+1] & 0x0F), Col: int(data[i+2] & 0x3F)}
		if w := d.CurrentWindow(); w != nil && w.Defined {
			if loc.Row < w.Rows && loc.Col < w.Cols {
				w.PenRow, w.PenCol = loc.Row, loc.Col
			}
		}
		d.emitCommand(Command{Kind: CmdSetPenLocation, PenLocation: loc, Raw: raw})

	case code == c1SWA:
		attr := decodeWindowAttributes(data[i+1:])
		d.emitCommand(Command{Kind: CmdSetWindowAttributes, WindowAttr: attr, Raw: raw})

	case code >= c1DF0 && code <= c1DF7:
		window := int(code - c1DF0)
		def := decodeWindowDef(data[i+1:])
		d.windows[window].define(def)
		if d.current < 0 {
			d.current = window
		}
		d.emitCommand(Command{Kind: CmdDefineWindow, Window: window, WindowDef: def, Raw: raw})

	default:
		// Reserved C1 positions (0x93..0x96) keep their raw byte.
		d.emitCommand(Command{Raw: raw})
	}

	return end
}

func (d *ServiceDecoder) applyWindowGroup(kind CmdKind, bitmap uint8) {
	for id := 0; id < MaxWindows; id++ {
		if bitmap&(1<<uint(id)) == 0 {
			continue
		}
		w := d.windows[id]
		switch kind {
		case CmdClearWindows:
			if w.Defined {
				w.clear()
			}
		case CmdDisplayWindows:
			w.Visible = true
		case CmdHideWindows:
			w.Visible = false
		case CmdToggleWindows:
			w.Visible = !w.Visible
		case CmdDeleteWindows:
			*w = *newWindow(id)
			if d.current == id {
				d.current = -1
			}
		}
	}
}

func decodeWindowDef(p []byte) WindowDef {
	return WindowDef{
		Priority:         int(p[0] & 0x07),
		ColumnLock:       p[0]&0x08 != 0,
		RowLock:          p[0]&0x10 != 0,
		Visible:          p[0]&0x20 != 0,
		RelativePosition: p[1]&0x80 != 0,
		AnchorVertical:   int(p[1] & 0x7F),
		AnchorHorizontal: int(p[2]),
		AnchorPoint:      int(p[3] >> 4),
		RowCount:         int(p[3]&0x0F) + 1,
		ColumnCount:      int(p[4]&0x3F) + 1,
		PenStyleID:       int(p[5] & 0x07),
		WindowStyleID:    int(p[5] >> 3),
	}
}

func decodePenAttributes(p1, p2 byte) PenAttributes {
	return PenAttributes{
		TextTag:   int(p1 & 0x0F),
		Offset:    int((p1 >> 4) & 0x03),
		PenSize:   int((p1 >> 6) & 0x03),
		Italic:    p2&0x01 != 0,
		Underline: p2&0x02 != 0,
		EdgeType:  int((p2 >> 2) & 0x07),
		FontStyle: int((p2 >> 5) & 0x07),
	}
}

func decodePenColor(p1, p2, p3 byte) PenColor {
	return PenColor{
		FgColor:   int(p1 & 0x3F),
		FgOpacity: int(p1 >> 6),
		BgColor:   int(p2 & 0x3F),
		BgOpacity: int(p2 >> 6),
		EdgeColor: int(p3 & 0x3F),
	}
}

func decodeWindowAttributes(p []byte) WindowAttributes {
	return WindowAttributes{
		FillColor:   int(p[0] & 0x3F),
		FillOpacity: int(p[0] >> 6),
		BorderColor: int(p[1] & 0x3F),
		BorderType:  int(p[2] & 0x07),
		WordWrap:    p[2]&0x08 != 0,
		PrintDir:    int((p[2] >> 4) & 0x03),
		ScrollDir:   int((p[2] >> 6) & 0x03),
		Justify:     int(p[3] & 0x03),
		EffectDir:   int((p[3] >> 2) & 0x03),
		EffectSpeed: int(p[3] >> 4),
	}
}

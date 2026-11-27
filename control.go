package cea608

func decodeControl(first, second byte) (Control, bool) {
	channel := channelOf(first)

	switch {
	case isControlLead(first) && second >= 0x20 && second <= 0x2f:
		code := dataPair(0x14, second)
		return Control{
			First: first, Second: second, Channel: channel,
			Code: code, Name: controlNames[second],
		}, true

	case (first == 0x11 || first == 0x19) && second >= 0x20 && second <= 0x2f:
		return Control{
			First: first, Second: second, Channel: channel,
			Code: dataPair(canonicalControlFirst(first), second),
			Name: "MIDROW",
		}, true

	case (first == 0x10 || first == 0x18) && second >= 0x20 && second <= 0x2f:
		return Control{
			First: first, Second: second, Channel: channel,
			Code: dataPair(canonicalControlFirst(first), second),
			Name: "BGA",
		}, true

	case (first == 0x17 || first == 0x1f) && second >= 0x2e && second <= 0x2f:
		return Control{
			First: first, Second: second, Channel: channel,
			Code: dataPair(canonicalControlFirst(first), second),
			Name: "BGA",
		}, true

	case (first == 0x17 || first == 0x1f) && second >= 0x21 && second <= 0x23:
		return Control{
			First: first, Second: second, Channel: channel,
			Code: dataPair(canonicalControlFirst(first), second),
			Name: "TAB",
		}, true
	}

	if first >= 0x10 && first <= 0x1f && second >= 0x40 && second <= 0x7f {
		index := ((first << 1) & 0x0e) | ((second >> 5) & 0x01)
		if pacRows[index] > 0 {
			return Control{
				First: first, Second: second, Channel: channel,
				Code: dataPair(first, second),
				Name: "PAC",
			}, true
		}
	}
	return Control{}, false
}

func decodeSpecial(first, second byte) (string, bool) {
	if isSpecialLead(first) && second >= 0x30 && second <= 0x3f {
		return string(specialCharacters[second-0x30]), true
	}
	return "", false
}

func decodeExtended(first, second byte) (string, bool) {
	if !isExtendedLead(first) || second < 0x20 || second > 0x3f {
		return "", false
	}
	table := &extendedSpanishFrench
	if first == 0x13 || first == 0x1b {
		table = &extendedPortugueseGerman
	}
	return string(table[second-0x20]), true
}

func (d *Decoder) acceptControl(control Control) []Event {
	if d.dupValid && d.dupCode == control.Code {
		return nil
	}
	d.dupValid = true
	d.dupCode = control.Code

	var caption *Caption
	switch control.Name {
	case "PAC":
		d.applyPAC(control.First, control.Second)
	case "TAB":
		d.cursorColumn += int(control.Second - 0x20)
		d.clampCursor()
	case "MIDROW", "BGA":
		// Styling and background attributes do not move the cursor.
	default:
		caption = d.applyCommand(control.Code)
	}

	events := []Event{{Type: EventControl, Control: &control}}
	if caption != nil {
		events = append(events, Event{Type: EventCaption, Caption: caption})
	}
	return events
}

func (d *Decoder) applyCommand(code ControlCode) *Caption {
	switch code {
	case ControlRCL:
		d.mode = ModeCaption
		d.target = d.hiddenMemory()
	case ControlBS:
		if d.cursorColumn > 0 {
			d.cursorColumn--
		}
	case ControlDER:
		d.clearToEndOfRow()
	case ControlRU2, ControlRU3, ControlRU4:
		d.mode = ModeRollUp
		d.target = displayed
		d.rollUpRows = int(code-ControlRU2) + 2
		if d.cursorRow < 0 || d.cursorRow > 14 {
			d.cursorRow = 14
		}
	case ControlRDC:
		d.mode = ModePaintOn
		d.target = displayed
	case ControlTR:
		d.mode = ModeText
		d.target = d.hiddenMemory()
	case ControlRTD:
		d.mode = ModeText
		d.target = displayed
	case ControlEDM:
		d.clearMemory(displayed)
		d.cursorColumn = 0
	case ControlCR:
		d.carriageReturn()
	case ControlENM:
		d.clearMemory(d.hiddenMemory())
		d.cursorColumn = 0
	case ControlEOC:
		return d.endOfCaption()
	}
	return nil
}

func (d *Decoder) applyPAC(first, second byte) {
	index := ((first << 1) & 0x0e) | ((second >> 5) & 0x01)
	row := pacRows[index] - 1
	if row < 0 {
		return
	}
	low := second & 0x1f
	indent := 0
	if low >= 0x12 {
		indent = int(low-0x10) * 2
	}
	d.cursorRow = row
	d.cursorColumn = indent
	d.clampCursor()
}

func (d *Decoder) carriageReturn() {
	if d.mode == ModeRollUp {
		rows := d.rollUpRows
		if rows < 1 {
			rows = 2
		}
		top := d.cursorRow - rows + 1
		if top < 0 {
			top = 0
		}
		mem := &d.memory[d.target]
		for row := top; row < d.cursorRow; row++ {
			mem[row] = mem[row+1]
		}
		for col := range mem[d.cursorRow] {
			mem[d.cursorRow][col] = ' '
		}
	} else if d.cursorRow < 14 {
		d.cursorRow++
	}
	d.cursorColumn = 0
}

func (d *Decoder) endOfCaption() *Caption {
	if d.target != d.displayed {
		oldDisplayed := d.displayed
		d.displayed = d.target
		d.target = oldDisplayed
	}
	caption := d.captureCaption()
	d.stats.Captions++
	if d.target != d.displayed {
		d.clearMemory(d.target)
	}
	d.cursorColumn = 0
	return &caption
}

func (d *Decoder) hiddenMemory() int {
	if d.displayed == displayed {
		return nonDisplayed
	}
	return displayed
}

func (d *Decoder) clearToEndOfRow() {
	row := &d.memory[d.target][d.cursorRow]
	for col := d.cursorColumn; col < len(row); col++ {
		row[col] = ' '
	}
}

func (d *Decoder) clearMemory(index int) {
	for row := range d.memory[index] {
		for col := range d.memory[index][row] {
			d.memory[index][row][col] = ' '
		}
	}
}

package cea608

import "strings"

// Displayed returns the currently displayed caption as text. Rows made only
// of spaces are omitted from the end, while interior layout is preserved.
func (d *Decoder) Displayed() string {
	return d.captureCaption().Text
}

// DisplayedCaption returns a snapshot of the currently displayed caption.
func (d *Decoder) DisplayedCaption() Caption {
	return d.captureCaption()
}

// DecodeText decodes a complete byte stream and returns the caption that is
// displayed at the end of it.
func DecodeText(p []byte) string {
	d := NewDecoder()
	d.Feed(p)
	return d.Displayed()
}

func (d *Decoder) writeText(text string) {
	for _, r := range text {
		d.writeCharacter(r)
	}
}

func (d *Decoder) writeCharacter(r rune) {
	if r == 0 {
		return
	}
	d.clampCursor()
	d.memory[d.target][d.cursorRow][d.cursorColumn] = r
	if d.cursorColumn < 31 {
		d.cursorColumn++
	}
}

func (d *Decoder) clampCursor() {
	if d.cursorRow < 0 {
		d.cursorRow = 0
	} else if d.cursorRow > 14 {
		d.cursorRow = 14
	}
	if d.cursorColumn < 0 {
		d.cursorColumn = 0
	} else if d.cursorColumn > 31 {
		d.cursorColumn = 31
	}
}

func (d *Decoder) captureCaption() Caption {
	rows := make([]string, 0, 15)
	last := -1
	for row := 0; row < 15; row++ {
		var b strings.Builder
		for _, r := range d.memory[d.displayed][row] {
			if r == 0 {
				r = ' '
			}
			b.WriteRune(r)
		}
		line := strings.TrimRight(b.String(), " ")
		rows = append(rows, line)
		if line != "" {
			last = row
		}
	}
	rows = rows[:last+1]
	return Caption{
		Text: strings.Join(rows, "\n"),
		Rows: rows,
	}
}

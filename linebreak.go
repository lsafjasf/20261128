package imscfit

import "strings"

// Line is one laid-out visual line of a cue.
type Line struct {
	Text   string  `json:"text"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	FontPx float64 `json:"fontPx"`
}

// Block is the laid-out block of one paragraph inside a content box.
type Block struct {
	Lines       []Line
	UsedW       float64
	UsedH       float64
	OverflowX   float64
	OverflowY   float64
	ForcedWidth bool // noWrap: horizontal extent is the text itself
}

type layoutEnv struct {
	contentW float64
	noWrap   bool
}

// LayOut wraps a paragraph into lines against the region content box.
func (d *Document) LayOut(p *Paragraph, props Props, g Geometry, c Canvas) Block {
	baseFont := d.resolveFont(props.FontSize, 0, c)
	lineHeight := d.resolveLineHeight(props, baseFont)
	env := layoutEnv{contentW: g.ContentW, noWrap: props.WrapOption == "noWrap"}

	atoms := d.buildAtoms(p, c)
	var textLines []textLine
	if env.noWrap {
		textLines = joinNoWrap(atoms)
	} else {
		textLines = wrapGreedy(atoms, env.contentW)
	}

	block := Block{ForcedWidth: env.noWrap}
	for _, tl := range textLines {
		w := textWidthPx(tl.text, tl.fontPx)
		block.Lines = append(block.Lines, Line{Text: tl.text, Width: w, Height: lineHeight, FontPx: tl.fontPx})
		if w > block.UsedW {
			block.UsedW = w
		}
	}
	block.UsedH = float64(len(block.Lines)) * lineHeight
	block.OverflowX = max64(0, block.UsedW-g.ContentW)
	block.OverflowY = max64(0, block.UsedH-g.ContentH)
	return block
}

func (d *Document) resolveLineHeight(props Props, baseFont float64) float64 {
	if props.LineHeight.Set {
		switch props.LineHeight.Unit {
		case UnitPX:
			return props.LineHeight.Value
		case UnitCell:
			return props.LineHeight.Value * baseFont
		default: // em / percent relative to font size
			return props.LineHeight.Value * baseFont
		}
	}
	return 1.25 * baseFont
}

type textLine struct {
	text   string
	fontPx float64
}

func joinNoWrap(atoms []atom) []textLine {
	lines := []textLine{}
	cur := strings.Builder{}
	fontPx := 0.0
	for _, a := range atoms {
		if a.breakk {
			lines = append(lines, textLine{cur.String(), fontPx})
			cur.Reset()
			fontPx = 0
			continue
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(string(a.runes))
		if fontPx == 0 {
			fontPx = a.fontPx
		}
	}
	lines = append(lines, textLine{cur.String(), fontPx})
	return lines
}

func wrapGreedy(atoms []atom, maxW float64) []textLine {
	lines := []textLine{}
	cur := strings.Builder{}
	curW := 0.0
	fontPx := 0.0

	flush := func() {
		lines = append(lines, textLine{cur.String(), fontPx})
		cur.Reset()
		curW = 0
		fontPx = 0
	}

	for _, a := range atoms {
		if a.breakk {
			flush()
			continue
		}
		word := string(a.runes)
		ww := textWidthPx(word, a.fontPx)
		spaceW := textWidthPx(" ", a.fontPx)

		if cur.Len() == 0 {
			cur.WriteString(word)
			curW = ww
			fontPx = a.fontPx
			if curW > maxW {
				curW = breakLongWord(&cur, word, a.fontPx, maxW, &lines)
				fontPx = a.fontPx
			}
			continue
		}
		if curW+spaceW+ww <= maxW {
			cur.WriteByte(' ')
			cur.WriteString(word)
			curW += spaceW + ww
			continue
		}
		flush()
		cur.WriteString(word)
		curW = ww
		fontPx = a.fontPx
		if curW > maxW {
			curW = breakLongWord(&cur, word, a.fontPx, maxW, &lines)
			fontPx = a.fontPx
		}
	}
	if cur.Len() > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

// breakLongWord splits an over-wide token character by character. CJK and
// other fullwidth runes break anywhere; Latin words break as a last resort.
func breakLongWord(cur *strings.Builder, word string, fontPx, maxW float64, lines *[]textLine) float64 {
	runes := []rune(word)
	cur.Reset()
	w := 0.0
	for _, r := range runes {
		rw := charWidthEm(r) * fontPx
		if w > 0 && w+rw > maxW {
			*lines = append(*lines, textLine{cur.String(), fontPx})
			cur.Reset()
			w = 0
		}
		cur.WriteRune(r)
		w += rw
	}
	return w
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

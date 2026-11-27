package subtitlelayout

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

type segment struct {
	text  string
	start int
}

type cluster struct {
	text  string
	width float64
	lead  rune
	start int
}

func splitSegments(text string) ([]segment, error) {
	segments := make([]segment, 0, 1)
	start := 0

	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			return nil, fmt.Errorf("%w at byte %d", ErrInvalidUTF8, i)
		}
		switch r {
		case '\n':
			segments = append(segments, segment{text: text[start:i], start: start})
			i += size
			start = i
		case '\r':
			segments = append(segments, segment{text: text[start:i], start: start})
			i += size
			if i < len(text) && text[i] == '\n' {
				i++
			}
			start = i
		default:
			i += size
		}
	}
	segments = append(segments, segment{text: text[start:], start: start})
	return segments, nil
}

func (l *Layouter) clusters(seg segment) ([]cluster, error) {
	result := make([]cluster, 0, len(seg.text))

	for i := 0; i < len(seg.text); {
		r, size := utf8.DecodeRuneInString(seg.text[i:])
		if r == utf8.RuneError && size == 1 {
			return nil, fmt.Errorf("%w at byte %d", ErrInvalidUTF8, seg.start+i)
		}

		if isCombiningMark(r) {
			if len(result) == 0 {
				result = append(result, cluster{
					text:  string(r),
					width: l.combiningWidth(r),
					lead:  r,
					start: seg.start + i,
				})
			} else {
				last := &result[len(result)-1]
				last.text += string(r)
				last.width += l.combiningWidth(r)
			}
			i += size
			continue
		}

		width, ok := l.widths[r]
		if !ok {
			return nil, fmt.Errorf("%w: U+%04X at byte %d", ErrUnknownGlyph, r, seg.start+i)
		}
		result = append(result, cluster{
			text:  string(r),
			width: width,
			lead:  r,
			start: seg.start + i,
		})
		i += size
	}

	return result, nil
}

func isCombiningMark(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Mc, r)
}

func (l *Layouter) combiningWidth(r rune) float64 {
	if unicode.Is(unicode.Mc, r) {
		if width, ok := l.widths[r]; ok {
			return width
		}
	}
	return 0
}

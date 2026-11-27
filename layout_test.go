package subtitlelayout

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func smallWidths() WidthTable {
	return WidthTable{
		'a': 1, 'b': 1, 'c': 1, 'd': 1, 'e': 1, 'x': 1, ' ': 1,
		'你': 2, '好': 2, '中': 2, '世': 2, '界': 2,
		'，': 2, '。': 2, ',': 1, '.': 1,
		'「': 2, '」': 2, '(': 1, ')': 1,
	}
}

func newTestLayouter(t *testing.T, widths WidthTable, opts Options) *Layouter {
	t.Helper()
	l, err := NewLayouter(widths, opts)
	if err != nil {
		t.Fatalf("NewLayouter() error = %v", err)
	}
	return l
}

func lineTexts(lines []Line) []string {
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = line.Text
	}
	return result
}

func assertLineTexts(t *testing.T, got []Line, want ...string) {
	t.Helper()
	texts := lineTexts(got)
	if strings.Join(texts, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("lines = %q, want %q", texts, want)
	}
}

func almostEqual(a, b float64) bool {
	scale := math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
	return math.Abs(a-b) <= widthTolerance*scale
}

func TestUsesWidthTableForFullAndHalfWidth(t *testing.T) {
	l := newTestLayouter(t, smallWidths(), Options{MaxWidth: 4})
	lines, err := l.Layout("中a中")
	if err != nil {
		t.Fatalf("Layout() error = %v", err)
	}
	assertLineTexts(t, lines, "中a", "中")
	if !almostEqual(lines[0].Width, 3) || !almostEqual(lines[1].Width, 2) {
		t.Fatalf("line widths = %v, %v; want 3, 2", lines[0].Width, lines[1].Width)
	}

	lines, err = l.Layout("abc中")
	if err != nil {
		t.Fatalf("Layout() error = %v", err)
	}
	assertLineTexts(t, lines, "abc", "中")
}

func TestCombiningMarksStayWithBaseAndNeedNoGlyphEntry(t *testing.T) {
	l := newTestLayouter(t, smallWidths(), Options{MaxWidth: 1})
	lines, err := l.Layout("e\u0301\u0308x")
	if err != nil {
		t.Fatalf("Layout() error = %v", err)
	}
	assertLineTexts(t, lines, "e\u0301\u0308", "x")
	if !almostEqual(lines[0].Width, 1) {
		t.Fatalf("first line width = %v, want 1", lines[0].Width)
	}
}

func TestSpacingCombiningMarkUsesTableWidth(t *testing.T) {
	const (
		base = '\u0915'
		mark = '\u093e'
	)
	widths := WidthTable{base: 1, mark: 0.5, 'x': 1}
	l := newTestLayouter(t, widths, Options{MaxWidth: 1.5})
	lines, err := l.Layout(string([]rune{base, mark, 'x'}))
	if err != nil {
		t.Fatalf("Layout() error = %v", err)
	}
	assertLineTexts(t, lines, string([]rune{base, mark}), "x")
	if !almostEqual(lines[0].Width, 1.5) {
		t.Fatalf("first line width = %v, want 1.5", lines[0].Width)
	}
}

func TestExplicitNewlinesArePreserved(t *testing.T) {
	l := newTestLayouter(t, smallWidths(), Options{MaxWidth: 2})
	lines, err := l.Layout("ab\r\n\r\ncd\ne")
	if err != nil {
		t.Fatalf("Layout() error = %v", err)
	}
	assertLineTexts(t, lines, "ab", "", "cd", "e")
	if lines[1].Width != 0 {
		t.Fatalf("blank line width = %v, want 0", lines[1].Width)
	}

	lines, err = l.Layout("x\n")
	if err != nil {
		t.Fatalf("Layout() error = %v", err)
	}
	assertLineTexts(t, lines, "x", "")
}

func TestClosingPunctuationDoesNotStartALine(t *testing.T) {
	l := newTestLayouter(t, smallWidths(), Options{MaxWidth: 4})
	lines, err := l.Layout("abcd，")
	if err != nil {
		t.Fatalf("Layout() error = %v", err)
	}
	assertLineTexts(t, lines, "abc", "d，")
}

func TestOpeningPunctuationDoesNotEndALine(t *testing.T) {
	l := newTestLayouter(t, smallWidths(), Options{MaxWidth: 4})
	lines, err := l.Layout("「abc」")
	if err != nil {
		t.Fatalf("Layout() error = %v", err)
	}
	assertLineTexts(t, lines, "「ab", "c」")
}

func TestMarginsReduceAvailableWidth(t *testing.T) {
	l := newTestLayouter(t, smallWidths(), Options{
		MaxWidth: 5, MarginLeft: 1, MarginRight: 1,
	})
	if !almostEqual(l.ContentWidth(), 3) {
		t.Fatalf("ContentWidth() = %v, want 3", l.ContentWidth())
	}
	lines, err := l.Layout("aaaa")
	if err != nil {
		t.Fatalf("Layout() error = %v", err)
	}
	assertLineTexts(t, lines, "aaa", "a")
	if lines[0].Width > l.ContentWidth() {
		t.Fatalf("line width = %v exceeds content width %v", lines[0].Width, l.ContentWidth())
	}
}

func TestConvenienceLayout(t *testing.T) {
	lines, err := Layout("abcd", smallWidths(), Options{MaxWidth: 2})
	if err != nil {
		t.Fatalf("Layout() error = %v", err)
	}
	assertLineTexts(t, lines, "ab", "cd")
}

func TestUnknownGlyphFails(t *testing.T) {
	l := newTestLayouter(t, smallWidths(), Options{MaxWidth: 10})
	_, err := l.Layout("a🙂")
	if !errors.Is(err, ErrUnknownGlyph) {
		t.Fatalf("Layout() error = %v, want ErrUnknownGlyph", err)
	}
}

func TestNegativeMarginsFail(t *testing.T) {
	widths := smallWidths()
	tests := []Options{
		{MaxWidth: 10, MarginLeft: -1},
		{MaxWidth: 10, MarginRight: -1},
	}
	for _, opts := range tests {
		_, err := NewLayouter(widths, opts)
		if !errors.Is(err, ErrNegativeMargin) {
			t.Fatalf("NewLayouter(%+v) error = %v, want ErrNegativeMargin", opts, err)
		}
	}
}

func TestTextThatCannotFitFails(t *testing.T) {
	widths := WidthTable{'中': 3}
	l := newTestLayouter(t, widths, Options{MaxWidth: 2})
	_, err := l.Layout("中")
	if !errors.Is(err, ErrTextDoesNotFit) {
		t.Fatalf("Layout() error = %v, want ErrTextDoesNotFit", err)
	}
}

func TestPunctuationRuleCanMakeTextUnlayoutable(t *testing.T) {
	widths := WidthTable{'「': 2, 'a': 1}
	l := newTestLayouter(t, widths, Options{MaxWidth: 2})
	_, err := l.Layout("「a")
	if !errors.Is(err, ErrTextDoesNotFit) {
		t.Fatalf("Layout() error = %v, want ErrTextDoesNotFit", err)
	}
}

func TestInvalidInputsFail(t *testing.T) {
	tests := []struct {
		name   string
		widths WidthTable
		opts   Options
		text   string
		want   error
	}{
		{name: "negative glyph width", widths: WidthTable{'a': -1}, opts: Options{MaxWidth: 10}, want: ErrInvalidWidth},
		{name: "NaN glyph width", widths: WidthTable{'a': math.NaN()}, opts: Options{MaxWidth: 10}, want: ErrInvalidWidth},
		{name: "negative max width", widths: WidthTable{}, opts: Options{MaxWidth: -1}, want: ErrInvalidWidth},
		{name: "invalid UTF-8", widths: smallWidths(), opts: Options{MaxWidth: 10}, text: string([]byte{0xff}), want: ErrInvalidUTF8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.text == "" {
				_, err := NewLayouter(tt.widths, tt.opts)
				if !errors.Is(err, tt.want) {
					t.Fatalf("NewLayouter() error = %v, want %v", err, tt.want)
				}
				return
			}
			l := newTestLayouter(t, tt.widths, tt.opts)
			_, err := l.Layout(tt.text)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Layout() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestEmptyTextProducesOneEmptyLine(t *testing.T) {
	l := newTestLayouter(t, smallWidths(), Options{MaxWidth: 1})
	lines, err := l.Layout("")
	if err != nil {
		t.Fatalf("Layout() error = %v", err)
	}
	assertLineTexts(t, lines, "")
	if lines[0].Width != 0 {
		t.Fatalf("empty line width = %v, want 0", lines[0].Width)
	}
}

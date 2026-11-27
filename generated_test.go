package subtitlelayout

import (
	"math"
	"math/rand"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func generatedWidthTable() WidthTable {
	table := make(WidthTable)
	for r := 'a'; r <= 'z'; r++ {
		table[r] = 0.5
	}
	for r := 'A'; r <= 'Z'; r++ {
		table[r] = 0.5
	}
	for _, r := range "0123456789" {
		table[r] = 0.5
	}
	for _, r := range "你好世界中文测试字体行布局" {
		table[r] = 1
	}
	for _, r := range "，。、；：！？「」『』（）【】" {
		table[r] = 1
	}
	for _, r := range ".,:;!?()[]" {
		table[r] = 0.5
	}
	table[' '] = 0.25
	table['\u00a0'] = 0.25
	return table
}

var generatedBaseTokens = []string{
	"a", "b", "c", "x", "y", "z",
	"A", "B", "C",
	"0", "1", "2",
	"你", "好", "世", "界", "中", "文", "测", "试", "字", "体", "行", "布", "局",
	"e\u0301", "a\u0308\u0327", "o\u0301",
	" ", "\u00a0",
}

var generatedClosingTokens = []string{
	"，", "。", "、", "；", "：", "！", "？",
	",", ".", ":", ";", "!", "?", "(", ")", "[", "]",
}

var generatedOpeningTokens = []string{
	"「", "『", "（", "【",
	"(", "[",
}

func generateText(rng *rand.Rand, tokenCount int) string {
	var b strings.Builder
	pausePunctuation := false
	for i := 0; i < tokenCount; i++ {
		b.WriteString(generatedBaseTokens[rng.Intn(len(generatedBaseTokens))])
		if pausePunctuation {
			pausePunctuation = false
			continue
		}
		if rng.Intn(4) == 0 {
			if rng.Intn(2) == 0 {
				b.WriteString(generatedOpeningTokens[rng.Intn(len(generatedOpeningTokens))])
			} else {
				b.WriteString(generatedClosingTokens[rng.Intn(len(generatedClosingTokens))])
			}
			pausePunctuation = true
		}
	}
	return b.String()
}

func measureText(t *testing.T, table WidthTable, text string) float64 {
	t.Helper()
	width := 0.0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if isCombiningMark(r) {
			i += size
			continue
		}
		w, ok := table[r]
		if !ok {
			t.Fatalf("generated text contains unmeasured glyph U+%04X", r)
		}
		width += w
		i += size
	}
	return width
}

func firstRune(text string) rune {
	r, _ := utf8.DecodeRuneInString(text)
	return r
}

func lastRune(text string) rune {
	r, _ := utf8.DecodeLastRuneInString(text)
	return r
}

func TestGeneratedAutoWrappedSamples(t *testing.T) {
	table := generatedWidthTable()
	rng := rand.New(rand.NewSource(20261128))
	contentWidths := []float64{2, 3.5, 5, 7.25}

	for _, contentWidth := range contentWidths {
		l := newTestLayouter(t, table, Options{
			MaxWidth:    contentWidth + 0.75,
			MarginLeft:  0.5,
			MarginRight: 0.25,
		})

		for sample := 0; sample < 250; sample++ {
			text := generateText(rng, 1+rng.Intn(100))
			lines, err := l.Layout(text)
			if err != nil {
				t.Fatalf("Layout(generated sample %d, width %v) error = %v", sample, contentWidth, err)
			}
			if got := strings.Join(lineTexts(lines), ""); got != text {
				t.Fatalf("generated text changed:\n got %q\nwant %q", got, text)
			}

			for i, line := range lines {
				if line.Text == "" {
					t.Fatalf("generated non-empty text produced an empty line")
				}
				if line.Width < 0 || line.Width > contentWidth+widthTolerance*math.Max(1, contentWidth) {
					t.Fatalf("line %d width %v exceeds content width %v", i, line.Width, contentWidth)
				}
				measured := measureText(t, table, line.Text)
				if !almostEqual(line.Width, measured) {
					t.Fatalf("line %d width = %v, measured %v", i, line.Width, measured)
				}
				if r := firstRune(line.Text); isCombiningMark(r) {
					t.Fatalf("line %d starts with a detached combining mark", i)
				}
				if i == 0 {
					continue
				}
				if cannotEndLine(lastRune(lines[i-1].Text)) {
					t.Fatalf("generated break leaves opening punctuation at line end: %q", lines[i-1].Text)
				}
				if cannotStartLine(firstRune(line.Text)) {
					t.Fatalf("generated break puts closing punctuation at line start: %q", line.Text)
				}
			}
		}
	}
}

func TestGeneratedHardBreakSamples(t *testing.T) {
	table := generatedWidthTable()
	rng := rand.New(rand.NewSource(77123))
	l := newTestLayouter(t, table, Options{MaxWidth: 6.5, MarginLeft: 0.5, MarginRight: 0.5})

	for sample := 0; sample < 200; sample++ {
		paragraphCount := 1 + rng.Intn(5)
		paragraphs := make([]string, paragraphCount)
		for i := range paragraphs {
			if rng.Intn(5) == 0 {
				paragraphs[i] = ""
				continue
			}
			paragraphs[i] = generateText(rng, 1+rng.Intn(25))
		}
		separators := []string{"\n", "\r\n", "\r"}
		separator := separators[rng.Intn(len(separators))]
		var input strings.Builder
		for i, paragraph := range paragraphs {
			if i > 0 {
				input.WriteString(separator)
			}
			input.WriteString(paragraph)
		}

		lines, err := l.Layout(input.String())
		if err != nil {
			t.Fatalf("Layout(generated hard-break sample %d) error = %v", sample, err)
		}

		want := make([]string, 0, len(paragraphs))
		for _, paragraph := range paragraphs {
			wrapped, err := l.Layout(paragraph)
			if err != nil {
				t.Fatalf("Layout(generated paragraph) error = %v", err)
			}
			want = append(want, lineTexts(wrapped)...)
		}
		if got := lineTexts(lines); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("hard breaks not preserved:\n got %q\nwant %q", got, want)
		}
		for i, line := range lines {
			if line.Width < 0 || line.Width > l.ContentWidth()+widthTolerance*math.Max(1, l.ContentWidth()) {
				t.Fatalf("line %d width %v exceeds content width %v", i, line.Width, l.ContentWidth())
			}
			if measured := measureText(t, table, line.Text); !almostEqual(line.Width, measured) {
				t.Fatalf("line %d width = %v, measured %v", i, line.Width, measured)
			}
		}
	}
}

func TestCombiningMarksDoNotHaveIndependentWidth(t *testing.T) {
	table := generatedWidthTable()
	for _, token := range generatedBaseTokens {
		r, _ := utf8.DecodeRuneInString(token)
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
			if _, ok := table[r]; ok {
				t.Fatalf("generated table unexpectedly assigns width to combining mark U+%04X", r)
			}
		}
	}
}

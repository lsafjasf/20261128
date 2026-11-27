package layout

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

func isWideRune(r rune) bool {
	return r >= 0x1100 && (r <= 0x115f ||
		r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f300 && r <= 0x1faff) ||
		(r >= 0x20000 && r <= 0x3fffd))
}

func isExtender(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) ||
		r == '\u200d' || r == '\ufe0e' || r == '\ufe0f' ||
		(r >= 0x1f3fb && r <= 0x1f3ff) ||
		(r >= 0xe0020 && r <= 0xe007f)
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1f1e6 && r <= 0x1f1ff
}

// graphemes is a compact, font-independent approximation of Unicode extended
// grapheme clusters. It covers combining marks, variation selectors, emoji
// modifiers, ZWJ sequences, flags, and keycaps.
func graphemes(text string) []string {
	if text == "" {
		return nil
	}
	runes := []rune(text)
	var result []string
	start := 0
	regionalRun := 0
	for i := 1; i <= len(runes); i++ {
		split := i == len(runes)
		if !split {
			prev := runes[i-1]
			current := runes[i]
			switch {
			case prev == '\u200d':
				split = false
			case isExtender(current):
				split = false
			case isRegionalIndicator(prev) && isRegionalIndicator(current):
				regionalRun++
				split = regionalRun%2 == 0
			default:
				split = true
			}
			if !isRegionalIndicator(current) {
				regionalRun = 0
			}
		}
		if split {
			result = append(result, string(runes[start:i]))
			start = i
		}
	}
	return result
}

// distributeClusters splits annotation text into contiguous pieces sized in
// proportion to base glyph widths. It never reorders or drops annotation
// clusters, which keeps wrapping lossless across line boundaries.
func distributeClusters(text string, weights []float64) []string {
	if len(weights) == 0 {
		return nil
	}
	clusters := graphemes(text)
	if len(weights) == 1 {
		return []string{text}
	}
	if len(clusters) == 0 {
		return make([]string, len(weights))
	}
	total := 0.0
	for _, weight := range weights {
		total += math.Max(0, weight)
	}
	if total == 0 {
		for i := range weights {
			weights[i] = 1
		}
		total = float64(len(weights))
	}
	result := make([]string, len(weights))
	start, cumulative := 0, 0.0
	for i, weight := range weights {
		cumulative += math.Max(0, weight)
		end := len(clusters)
		if i < len(weights)-1 {
			end = int(math.Round(cumulative / total * float64(len(clusters))))
			if end < start {
				end = start
			}
			remaining := len(weights) - i - 1
			if len(clusters)-end < remaining {
				end = len(clusters) - remaining
				if end < start {
					end = start
				}
			}
		}
		result[i] = strings.Join(clusters[start:end], "")
		start = end
	}
	return result
}

func splitTextRun(text string) []inlineItem {
	var items []inlineItem
	var word strings.Builder
	flushWord := func() {
		if word.Len() == 0 {
			return
		}
		items = append(items, inlineItem{kind: itemText, text: word.String()})
		word.Reset()
	}
	for _, cluster := range graphemes(text) {
		r, _ := utf8.DecodeRuneInString(cluster)
		if unicode.IsSpace(r) {
			flushWord()
			if len(items) == 0 || items[len(items)-1].text != " " {
				items = append(items, inlineItem{kind: itemText, text: " "})
			}
			continue
		}
		if isWideRune(r) {
			flushWord()
			items = append(items, inlineItem{kind: itemText, text: cluster})
			continue
		}
		word.WriteString(cluster)
	}
	flushWord()
	return items
}

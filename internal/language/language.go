// Package language classifies subtitle text by writing script so that the
// planner can keep different languages on separate visual regions instead of
// concatenating them into one line.
package language

import (
	"regexp"
	"unicode"
)

// Language codes used by the planner.
const (
	Chinese  = "zh"
	English  = "en"
	Japanese = "ja"
	Korean   = "ko"
	Russian  = "ru"
	Arabic   = "ar"
	Unknown  = "und"
)

var (
	hangulRe    = regexp.MustCompile(`\p{Hangul}`)
	kanaRe      = regexp.MustCompile(`[\p{Hiragana}\p{Katakana}]`)
	hanRe       = regexp.MustCompile(`\p{Han}`)
	cyrillicRe  = regexp.MustCompile(`\p{Cyrillic}`)
	arabicRe    = regexp.MustCompile(`\p{Arabic}`)
	latinWordRe = regexp.MustCompile(`[A-Za-z]+`)
)

// Detect inspects the letter-bearing runes and returns the dominant language
// code. Japanese is distinguished from Chinese by the presence of kana,
// Korean by Hangul jamo; Latin text defaults to English.
func Detect(text string) string {
	score := map[string]int{}
	for _, r := range text {
		switch {
		case !unicode.IsLetter(r):
			continue
		case hangulRe.MatchString(string(r)):
			score[Korean] += 2
		case kanaRe.MatchString(string(r)):
			score[Japanese] += 2
		case hanRe.MatchString(string(r)):
			score[Chinese]++
		case cyrillicRe.MatchString(string(r)):
			score[Russian]++
		case arabicRe.MatchString(string(r)):
			score[Arabic]++
		case r >= 'A' && r <= 'z':
			score[English]++
		}
	}

	// Kana unambiguously marks Japanese even when Han characters dominate.
	if score[Japanese] > 0 {
		return Japanese
	}
	if score[Korean] > 0 {
		return Korean
	}
	if score[Arabic] > 0 && score[Arabic] >= score[Chinese] &&
		score[Arabic] >= score[English] {
		return Arabic
	}
	if score[Russian] > 0 && score[Russian] >= score[English] {
		return Russian
	}
	if score[Chinese] > 0 {
		return Chinese
	}
	if score[English] > 0 {
		return English
	}
	return Unknown
}

// Same reports whether two detected codes describe the same language family.
// Unknown only matches unknown; every concrete code only matches itself.
func Same(a, b string) bool {
	return a == b
}

// LatinWordCount supports heuristics such as minimum fragment length.
func LatinWordCount(text string) int {
	return len(latinWordRe.FindAllString(text, -1))
}

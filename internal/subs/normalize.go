package subs

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	tagRe        = regexp.MustCompile(`<[^>]+>`)
	voiceRe      = regexp.MustCompile(`^\s*<v[^>]*>`)
	braceRe      = regexp.MustCompile(`\{[^}]*\}`)
	spaceRe      = regexp.MustCompile(`[ \t]+`)
	multiBreakRe = regexp.MustCompile(`\n{2,}`)
	langSuffixRe = regexp.MustCompile(`(?i)(?:^|[.\-_])([a-z]{2,3}(?:-[a-z0-9]{2,8})*)$`)
)

// iso6393Aliases maps common three-letter language codes used in real-world
// file naming to their two-letter ISO 639-1 counterpart, so eng/en and
// fra/fr tracks group together.
var iso6393Aliases = map[string]string{
	"eng": "en", "fra": "fr", "fre": "fr", "deu": "de", "ger": "de",
	"spa": "es", "zho": "zh", "chi": "zh", "jpn": "ja", "kor": "ko",
	"ita": "it", "por": "pt", "rus": "ru", "ara": "ar", "nld": "nl",
	"pol": "pl", "swe": "sv", "tur": "tr", "hin": "hi", "vie": "vi",
	"tha": "th", "ind": "id", "ces": "cs", "dan": "da", "fin": "fi",
	"ell": "el", "heb": "he", "nor": "no", "ron": "ro", "ukr": "uk",
}

// candidateFrameRates covers cinema, broadcast and common PAL/NTSC grids.
var candidateFrameRates = []float64{
	23.976, 24, 25, 29.97, 30, 48, 50, 59.94, 60, 1000,
}

// NormalizeText strips markup, voice labels and redundant whitespace so that
// the same translation compares identical across containers.
func NormalizeText(raw string) string {
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = voiceRe.ReplaceAllString(s, "")
	s = tagRe.ReplaceAllString(s, "")
	s = braceRe.ReplaceAllString(s, "")
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = spaceRe.ReplaceAllString(strings.TrimSpace(line), " ")
		if line != "" {
			lines = append(lines, line)
		}
	}
	out := strings.Join(lines, "\n")
	out = multiBreakRe.ReplaceAllString(out, "\n")
	return strings.TrimSpace(out)
}

// inferFrameRate finds the smallest candidate frame grid that every
// millisecond timestamp lands on, within a 1 ms quantization tolerance.
func inferFrameRate(doc *Document) {
	if doc.Format == "ass" {
		// ASS timestamps are centiseconds by definition; no frame grid.
		return
	}
	ms := make([]int64, 0, len(doc.Cues)*2)
	for _, cue := range doc.Cues {
		ms = append(ms, cue.Start.Milliseconds(), cue.End.Milliseconds())
	}
	for _, fps := range candidateFrameRates {
		step := 1000.0 / fps
		allFit := true
		for _, t := range ms {
			r := math.Mod(float64(t), step)
			if r > 1.0 && r < step-1.0 {
				allFit = false
				break
			}
		}
		if allFit && fps != 1000 {
			doc.Timebase.FrameRate = fps
			doc.Timebase.Source = doc.Timebase.Source + " (inferred " +
				strconv.FormatFloat(fps, 'f', -1, 64) + " fps grid)"
			return
		}
	}
	doc.Timebase.FrameRate = 0
}

// LanguageFromName extracts a BCP-47-ish tag from a filename stem, e.g.
// "movie.zh-Hans" -> "zh-Hans", "clip_en" -> "en".
func LanguageFromName(name string) string {
	stem := name
	if i := strings.LastIndex(stem, "."); i >= 0 {
		stem = stem[:i]
	}
	if m := langSuffixRe.FindStringSubmatch(stem); m != nil {
		return normalizeLang(m[1])
	}
	return ""
}

func normalizeLang(lang string) string {
	parts := strings.Split(lang, "-")
	for i, p := range parts {
		if i == 0 {
			base := strings.ToLower(p)
			if alias, ok := iso6393Aliases[base]; ok {
				base = alias
			}
			parts[i] = base
			continue
		}
		if len(p) == 2 {
			parts[i] = strings.ToUpper(p)
		} else {
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, "-")
}

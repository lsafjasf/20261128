package subs

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var assTimeRe = regexp.MustCompile(`(\d{1,2}):(\d{2}):(\d{2})\.(\d{2})`)
var assLangRe = regexp.MustCompile(`(?i)^(?:Language|WrapStyle)\s*:\s*([A-Za-z]{2,3}(?:-[A-Za-z0-9]+)*)`)
var assOverrideRe = regexp.MustCompile(`\{[^}]*\}`)

func parseASS(path, text string) (*Document, error) {
	doc := &Document{Path: path, Format: "ass", Timebase: Timebase{Scale: "100", Source: "ass centisecond grid"}}
	sc := newScanner(text)
	var formatCols []string
	section := ""
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = trimmed
			continue
		}
		if section == "[Script Info]" {
			if m := assLangRe.FindStringSubmatch(trimmed); m != nil {
				doc.Language = normalizeLang(m[1])
			}
			continue
		}
		if section != "[Events]" {
			continue
		}
		if strings.HasPrefix(trimmed, "Format:") {
			formatCols = splitASSFormat(trimmed)
			continue
		}
		if !strings.HasPrefix(trimmed, "Dialogue:") || len(formatCols) == 0 {
			continue
		}
		fields := splitASSDialogue(strings.TrimPrefix(trimmed, "Dialogue:"), len(formatCols))
		if len(fields) < len(formatCols) {
			continue
		}
		start, ok1 := parseASSTime(fields[indexOf(formatCols, "Start")])
		end, ok2 := parseASSTime(fields[indexOf(formatCols, "End")])
		if !ok1 || !ok2 {
			continue
		}
		body := fields[indexOf(formatCols, "Text")]
		body = assOverrideRe.ReplaceAllString(body, "")
		body = strings.ReplaceAll(body, `\N`, "\n")
		body = strings.ReplaceAll(body, `\n`, "\n")
		body = strings.ReplaceAll(body, `\h`, " ")
		body = NormalizeText(body)
		if body == "" {
			continue
		}
		doc.Cues = append(doc.Cues, Cue{Start: start, End: end, Text: body})
	}
	return doc, nil
}

func splitASSFormat(line string) []string {
	parts := strings.Split(strings.TrimPrefix(line, "Format:"), ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func splitASSDialogue(rest string, n int) []string {
	// Text is the last column and may itself contain commas, so split only
	// the first n-1 structural fields.
	parts := strings.SplitN(rest, ",", n)
	out := make([]string, 0, n)
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func indexOf(cols []string, name string) int {
	for i, c := range cols {
		if strings.EqualFold(c, name) {
			return i
		}
	}
	return len(cols) - 1
}

func parseASSTime(v string) (time.Duration, bool) {
	m := assTimeRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return 0, false
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	s, _ := strconv.Atoi(m[3])
	cs, _ := strconv.Atoi(m[4])
	return time.Duration(h)*time.Hour + time.Duration(min)*time.Minute +
		time.Duration(s)*time.Second + time.Duration(cs)*10*time.Millisecond, true
}

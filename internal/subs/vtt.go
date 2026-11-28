package subs

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var vttLangRe = regexp.MustCompile(`(?i)language[=:]\s*([A-Za-z]{2,3}(?:-[A-Za-z0-9]+)*)`)
var vttOffsetRe = regexp.MustCompile(`(?i)X-TIMESTAMP-MAP[^\n]*`)
var vttLocalRe = regexp.MustCompile(`(?i)LOCAL:((?:(\d{1,2}):)?(\d{1,2}):(\d{2})\.(\d{1,3}))`)

func parseVTT(path, text string) (*Document, error) {
	doc := &Document{Path: path, Format: "vtt", Timebase: Timebase{Scale: "1000", Source: "webvtt millisecond grid"}}
	if m := vttLangRe.FindStringSubmatch(text); m != nil {
		doc.Language = normalizeLang(m[1])
	}
	if m := vttOffsetRe.FindString(text); m != "" {
		if lm := vttLocalRe.FindStringSubmatch(m); lm != nil {
			if d, ok := parseVTTClock(lm[1]); ok {
				doc.Timebase.Offset = d
				doc.Timebase.Source = "webvtt millisecond grid + X-TIMESTAMP-MAP"
			}
		}
	}

	sc := newScanner(text)
	var lines []string
	inHeader := true
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if inHeader {
			// The header ends at the first blank line after WEBVTT.
			if trimmed == "" {
				inHeader = false
			}
			continue
		}
		if trimmed == "" {
			if err := appendBlock(doc, lines, vttTimeRe, parseVTTGroups); err != nil {
				return nil, err
			}
			lines = lines[:0]
			continue
		}
		// Skip cue identifiers (a line that is not a timestamp and precedes one).
		lines = append(lines, line)
	}
	if err := appendBlock(doc, lines, vttTimeRe, parseVTTGroups); err != nil {
		return nil, err
	}
	if doc.Timebase.Offset != 0 {
		for i := range doc.Cues {
			doc.Cues[i].Start += doc.Timebase.Offset
			doc.Cues[i].End += doc.Timebase.Offset
		}
	}
	inferFrameRate(doc)
	return doc, nil
}

// parseVTTGroups consumes the four colon-joined groups produced by vttTimeRe:
// hour (may be empty), minute, second and millisecond fraction.
func parseVTTGroups(joined string) (time.Duration, bool) {
	parts := strings.Split(joined, ":")
	if len(parts) != 4 {
		return 0, false
	}
	if parts[0] == "" {
		parts[0] = "0"
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, false
	}
	s, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, false
	}
	ms, err := strconv.Atoi(padFrac(parts[3], 3))
	if err != nil {
		return 0, false
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute +
		time.Duration(s)*time.Second + time.Duration(ms)*time.Millisecond, true
}

// parseVTTClock parses standalone clocks used in X-TIMESTAMP-MAP headers.
func parseVTTClock(joined string) (time.Duration, bool) {
	parts := strings.Split(joined, ":")
	switch len(parts) {
	case 2:
		return parseVTTGroups(":" + strings.Join(parts, ":"))
	case 3:
		return parseVTTGroups(strings.Join(parts, ":"))
	default:
		return 0, false
	}
}

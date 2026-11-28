package subs

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrEmpty means the file contained no parseable cues.
var ErrEmpty = errors.New("subs: no cues found")

var srtTimeRe = regexp.MustCompile(`(\d{1,2}):(\d{2}):(\d{2})[,.](\d{1,3})\s*-->\s*(\d{1,2}):(\d{2}):(\d{2})[,.](\d{1,3})`)

// Four groups per timestamp (hour, minute, second, fraction); the hour group
// is empty for mm:ss.fff clocks.
var vttTimeRe = regexp.MustCompile(`(\d{1,2})?:?(\d{1,2}):(\d{2})\.(\d{1,3})\s*-->\s*(\d{1,2})?:?(\d{1,2}):(\d{2})\.(\d{1,3})`)

// Parse reads a subtitle file, detecting its format from header/extension.
func Parse(path string) (*Document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := parseBytes(path, raw)
	if err != nil {
		return nil, err
	}
	doc.Path = path
	if doc.Language == "" {
		doc.Language = LanguageFromName(filepath.Base(path))
	}
	if len(doc.Cues) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrEmpty, path)
	}
	return doc, nil
}

func parseBytes(path string, raw []byte) (*Document, error) {
	text := string(raw)
	// Strip UTF-8 BOM; all parsers are line based.
	text = strings.TrimPrefix(text, "\ufeff")
	switch {
	case strings.HasPrefix(text, "WEBVTT"):
		return parseVTT(path, text)
	case strings.Contains(text, "[Script Info]") && strings.Contains(text, "[V4+ Styles]"):
		return parseASS(path, text)
	default:
		if srtTimeRe.MatchString(text) {
			return parseSRT(path, text)
		}
		if strings.HasPrefix(text, "WEBVTT") || vttTimeRe.MatchString(text) {
			return parseVTT(path, text)
		}
		return nil, fmt.Errorf("subs: unsupported or unrecognized subtitle format: %s", path)
	}
}

func newScanner(text string) *bufio.Scanner {
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	return sc
}

func parseSRT(path, text string) (*Document, error) {
	doc := &Document{Path: path, Format: "srt", Timebase: Timebase{Scale: "1000", Source: "srt millisecond grid"}}
	sc := newScanner(text)
	var lines []string
	flush := func() error {
		return appendBlock(doc, lines, srtTimeRe, parseSRTTime)
	}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			lines = lines[:0]
			continue
		}
		lines = append(lines, line)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	inferFrameRate(doc)
	return doc, nil
}

func appendBlock(doc *Document, lines []string, re *regexp.Regexp, parseTime func(string) (time.Duration, bool)) error {
	if len(lines) == 0 {
		return nil
	}
	idx := -1
	for i, line := range lines {
		if re.MatchString(stripSettings(line)) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	m := re.FindStringSubmatch(stripSettings(lines[idx]))
	groups := m[1:]
	half := len(groups) / 2
	start, ok1 := parseTime(strings.Join(groups[:half], ":"))
	end, ok2 := parseTime(strings.Join(groups[half:], ":"))
	if !ok1 || !ok2 {
		return fmt.Errorf("subs: bad timestamp in %s: %q", doc.Path, lines[idx])
	}
	text := NormalizeText(strings.Join(lines[idx+1:], "\n"))
	if text == "" {
		return nil
	}
	doc.Cues = append(doc.Cues, Cue{Start: start, End: end, Text: text})
	return nil
}

func stripSettings(line string) string {
	if i := strings.Index(line, " --> "); i >= 0 {
		return line
	}
	return line
}

func parseSRTTime(joined string) (time.Duration, bool) {
	parts := strings.Split(joined, ":")
	if len(parts) != 4 {
		return 0, false
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	s, _ := strconv.Atoi(parts[2])
	frac := parts[3]
	ms := padFrac(frac, 3)
	n, err := strconv.Atoi(ms)
	if err != nil {
		return 0, false
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second + time.Duration(n)*time.Millisecond, true
}

func padFrac(frac string, width int) string {
	if len(frac) >= width {
		return frac[:width]
	}
	return frac + strings.Repeat("0", width-len(frac))
}

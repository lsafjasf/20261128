package webvtt

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Parse parses a complete WebVTT document.
func Parse(data []byte) (*Document, error) {
	return ParseString(string(data))
}

// Read parses a WebVTT document from r.
func Read(r io.Reader) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("webvtt: read document: %w", err)
	}
	return Parse(data)
}

// ParseString parses a complete WebVTT document.
func ParseString(source string) (*Document, error) {
	source = strings.TrimPrefix(source, "\ufeff")
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	lines := strings.Split(source, "\n")
	if len(lines) == 0 || !isHeaderLine(lines[0]) {
		return nil, fmt.Errorf("webvtt: missing WEBVTT header")
	}
	doc := &Document{Header: lines[0]}

	pos := 1
	for pos < len(lines) && lines[pos] != "" {
		pos++
	}
	for pos < len(lines) && lines[pos] == "" {
		pos++
	}
	for pos < len(lines) {
		start := pos
		for pos < len(lines) && lines[pos] != "" {
			pos++
		}
		block := lines[start:pos]
		for pos < len(lines) && lines[pos] == "" {
			pos++
		}
		if len(block) == 0 || isSkippedBlock(block[0]) {
			continue
		}
		cue, err := parseCueBlock(block)
		if err != nil {
			return nil, err
		}
		if cue != nil {
			doc.Cues = append(doc.Cues, *cue)
		}
	}
	return doc, nil
}

func isHeaderLine(line string) bool {
	if line == "WEBVTT" {
		return true
	}
	return strings.HasPrefix(line, "WEBVTT ") || strings.HasPrefix(line, "WEBVTT\t")
}

func isSkippedBlock(first string) bool {
	return first == "STYLE" || first == "REGION" ||
		first == "NOTE" || strings.HasPrefix(first, "NOTE ") ||
		strings.HasPrefix(first, "NOTE\t")
}

func parseCueBlock(lines []string) (*Cue, error) {
	var cue Cue
	timingIndex := -1
	if strings.Contains(lines[0], "-->") {
		timingIndex = 0
	} else if len(lines) > 1 && strings.Contains(lines[1], "-->") {
		cue.ID = lines[0]
		timingIndex = 1
	} else {
		return nil, nil
	}

	start, end, settings, err := parseTimingLine(lines[timingIndex])
	if err != nil {
		return nil, err
	}
	cue.Start = start
	cue.End = end
	cue.Settings = settings
	if timingIndex+1 < len(lines) {
		cue.Payload = strings.Join(lines[timingIndex+1:], "\n")
		cue.Nodes = ParseCueMarkup(cue.Payload)
	}
	return &cue, nil
}

func parseTimingLine(line string) (time.Duration, time.Duration, string, error) {
	arrow := strings.Index(line, "-->")
	if arrow < 0 {
		return 0, 0, "", fmt.Errorf("webvtt: invalid cue timing %q", line)
	}
	startText := strings.TrimSpace(line[:arrow])
	right := strings.TrimSpace(line[arrow+3:])
	if startText == "" || right == "" {
		return 0, 0, "", fmt.Errorf("webvtt: invalid cue timing %q", line)
	}
	endText := right
	settings := ""
	if split := strings.IndexAny(right, " \t"); split >= 0 {
		endText = right[:split]
		settings = strings.TrimSpace(right[split:])
	}
	start, err := parseTimestamp(startText)
	if err != nil {
		return 0, 0, "", fmt.Errorf("webvtt: cue start: %w", err)
	}
	end, err := parseTimestamp(endText)
	if err != nil {
		return 0, 0, "", fmt.Errorf("webvtt: cue end: %w", err)
	}
	if end < start {
		return 0, 0, "", fmt.Errorf("webvtt: cue ends before it starts")
	}
	return start, end, settings, nil
}

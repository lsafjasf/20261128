package ass

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Parse reads an ASS script. Only Script Info, the V4+ event Format line, and
// Dialogue entries are required; unrelated sections are ignored.
func Parse(r io.Reader) (*Script, error) {
	script := &Script{
		Info: make(map[string]string),
		Style: Style{
			PlayResX: DefaultPlayResX,
			PlayResY: DefaultPlayResY,
		},
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	section := ""
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if lineNumber == 1 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.ToLower(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
			continue
		}

		switch section {
		case "script info":
			parseInfoLine(script, trimmed)
		case "events":
			if err := parseEventLine(script, trimmed, lineNumber); err != nil {
				return nil, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("ass: read script: %w", err)
	}
	if script.Style.PlayResX <= 0 {
		script.Style.PlayResX = DefaultPlayResX
	}
	if script.Style.PlayResY <= 0 {
		script.Style.PlayResY = DefaultPlayResY
	}
	return script, nil
}

// ParseString parses an ASS document held in s.
func ParseString(s string) (*Script, error) {
	return Parse(strings.NewReader(s))
}

func parseInfoLine(script *Script, line string) {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	script.Info[strings.ToLower(key)] = value
	switch strings.ToLower(key) {
	case "playresx":
		if n, err := strconv.Atoi(value); err == nil {
			script.Style.PlayResX = n
		}
	case "playresy":
		if n, err := strconv.Atoi(value); err == nil {
			script.Style.PlayResY = n
		}
	}
}

func parseEventLine(script *Script, line string, lineNumber int) error {
	kind, rest, ok := strings.Cut(line, ":")
	if !ok {
		return nil
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "format" {
		fields := splitCSV(rest)
		script.Format = script.Format[:0]
		for _, field := range fields {
			script.Format = append(script.Format, strings.ToLower(strings.TrimSpace(field)))
		}
		return nil
	}
	if kind != "dialogue" {
		return nil
	}
	if len(script.Format) == 0 {
		script.Format = []string{
			"layer", "start", "end", "style", "name",
			"marginl", "marginr", "marginv", "effect", "text",
		}
	}
	fields := splitCSVLimit(rest, len(script.Format))
	if len(fields) < len(script.Format) {
		return fmt.Errorf("ass: line %d: dialogue has %d fields, want %d", lineNumber, len(fields), len(script.Format))
	}
	event := Event{Line: lineNumber}
	for i, name := range script.Format {
		value := strings.TrimSpace(fields[i])
		switch name {
		case "layer":
			event.Layer, _ = strconv.Atoi(value)
		case "start":
			start, err := ParseTimestamp(value)
			if err != nil {
				return fmt.Errorf("ass: line %d: invalid start: %w", lineNumber, err)
			}
			event.Start = start
		case "end":
			end, err := ParseTimestamp(value)
			if err != nil {
				return fmt.Errorf("ass: line %d: invalid end: %w", lineNumber, err)
			}
			event.End = end
		case "style":
			event.Style = value
		case "name":
			event.Name = value
		case "marginl":
			event.MarginL, _ = strconv.Atoi(value)
		case "marginr":
			event.MarginR, _ = strconv.Atoi(value)
		case "marginv":
			event.MarginV, _ = strconv.Atoi(value)
		case "effect":
			event.Effect = value
		case "text":
			event.Text = fields[i]
		}
	}
	if event.End < event.Start {
		return fmt.Errorf("ass: line %d: end precedes start", lineNumber)
	}
	script.Events = append(script.Events, event)
	return nil
}

func splitCSV(s string) []string {
	return strings.Split(s, ",")
}

func splitCSVLimit(s string, count int) []string {
	if count <= 0 {
		return nil
	}
	fields := make([]string, 0, count)
	for len(fields) < count-1 {
		index := strings.IndexByte(s, ',')
		if index < 0 {
			break
		}
		fields = append(fields, s[:index])
		s = s[index+1:]
	}
	fields = append(fields, s)
	return fields
}

// ParseTimestamp parses ASS's H:MM:SS.cc timestamp.
func ParseTimestamp(value string) (time.Duration, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("timestamp %q must be H:MM:SS.cc", value)
	}
	hours, err := strconv.Atoi(parts[0])
	if err != nil || hours < 0 {
		return 0, fmt.Errorf("timestamp %q has invalid hours", value)
	}
	minutes, err := strconv.Atoi(parts[1])
	if err != nil || minutes < 0 || minutes > 59 {
		return 0, fmt.Errorf("timestamp %q has invalid minutes", value)
	}
	seconds, fraction, _ := strings.Cut(parts[2], ".")
	sec, err := strconv.Atoi(seconds)
	if err != nil || sec < 0 || sec > 59 {
		return 0, fmt.Errorf("timestamp %q has invalid seconds", value)
	}
	millis := 0
	if fraction != "" {
		if len(fraction) > 3 {
			fraction = fraction[:3]
		}
		fraction += strings.Repeat("0", 3-len(fraction))
		millis, err = strconv.Atoi(fraction)
		if err != nil {
			return 0, fmt.Errorf("timestamp %q has invalid fraction", value)
		}
	}
	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(sec)*time.Second +
		time.Duration(millis)*time.Millisecond, nil
}

// FormatTimestamp renders a WebVTT timestamp.
func FormatTimestamp(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	totalMillis := value.Milliseconds()
	hours := totalMillis / 3_600_000
	totalMillis %= 3_600_000
	minutes := totalMillis / 60_000
	totalMillis %= 60_000
	seconds := totalMillis / 1_000
	millis := totalMillis % 1_000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

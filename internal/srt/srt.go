// Package srt parses and renders SubRip (.srt) subtitle files.
//
// The parser tolerates BOMs, CRLF line endings, missing sequence numbers and
// the widely used coordinate extension on the timestamp line
// ("... --> ... X1:0 Y1:0 X2:720 Y2:60").
package srt

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Region is the vertical placement declared by a cue, if any.
type Region int

const (
	RegionUnspecified Region = iota
	RegionTop
	RegionBottom
)

func (r Region) String() string {
	switch r {
	case RegionTop:
		return "top"
	case RegionBottom:
		return "bottom"
	default:
		return "unspecified"
	}
}

// Position holds the optional pixel coordinates attached to a timestamp.
type Position struct {
	Has    bool
	X1, Y1 int
	X2, Y2 int
	Raw    string
}

// Cue is one parsed subtitle entry.
type Cue struct {
	Seq      int
	Start    time.Duration
	End      time.Duration
	Lines    []string
	Position Position
}

// File groups the cues read from one .srt input.
type File struct {
	Path string
	Cues []Cue
}

var (
	timestampRe = regexp.MustCompile(
		`^(\d{1,2}):(\d{2}):(\d{2})[,.](\d{1,3})\s*-->\s*` +
			`(\d{1,2}):(\d{2}):(\d{2})[,.](\d{1,3})\s*(.*)$`)
	coordRe = regexp.MustCompile(`(X1|Y1|X2|Y2)\s*:\s*(\d+)`)
)

// ParseTime decodes an SRT timecode "HH:MM:SS,mmm" (dot also accepted).
func ParseTime(value string) (time.Duration, error) {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool {
		return r == ':' || r == ',' || r == '.'
	})
	if len(parts) != 4 {
		return 0, fmt.Errorf("srt: invalid timecode %q", value)
	}
	nums := make([]int, 4)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, fmt.Errorf("srt: invalid timecode %q", value)
		}
		nums[i] = n
	}
	ms := padMillis(parts[3], nums[3])
	d := time.Duration(nums[0])*time.Hour +
		time.Duration(nums[1])*time.Minute +
		time.Duration(nums[2])*time.Second +
		time.Duration(ms)*time.Millisecond
	return d, nil
}

// padMillis expands a 1-2 digit millisecond field to three digits without
// misreading already-padded values such as "004" (4 ms, not 400 ms).
func padMillis(raw string, value int) int {
	switch len(raw) {
	case 1:
		return value * 100
	case 2:
		return value * 10
	default:
		return value
	}
}

// FormatTime encodes a duration as "HH:MM:SS,mmm".
func FormatTime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	ms := d.Milliseconds()
	h := ms / 3_600_000
	ms %= 3_600_000
	m := ms / 60_000
	ms %= 60_000
	s := ms / 1_000
	ms %= 1_000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// Parse reads all cues from r.
func Parse(r io.Reader) (*File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	data = trimBOM(data)
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	f := &File{}
	autoSeq := 0
	for _, block := range strings.Split(text, "\n\n") {
		lines := strings.Split(block, "\n")
		for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
			lines = lines[1:]
		}
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		if len(lines) == 0 {
			continue
		}
		autoSeq++
		cue, err := parseBlock(lines, autoSeq)
		if err != nil {
			return nil, err
		}
		if cue.Seq == 0 {
			cue.Seq = autoSeq
		}
		f.Cues = append(f.Cues, cue)
	}
	return f, nil
}

func trimBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

func parseBlock(lines []string, autoSeq int) (Cue, error) {
	idx := 0
	cue := Cue{Seq: autoSeq}
	if seq, err := strconv.Atoi(strings.TrimSpace(lines[0])); err == nil {
		cue.Seq = seq
		idx = 1
	}
	if idx >= len(lines) {
		return cue, fmt.Errorf("srt: cue %d is missing a timestamp line", autoSeq)
	}
	m := timestampRe.FindStringSubmatch(lines[idx])
	if m == nil {
		return cue, fmt.Errorf("srt: cue %d has an unparseable timestamp line %q", autoSeq, lines[idx])
	}
	start, err := parseTimestampParts(m[1], m[2], m[3], m[4])
	if err != nil {
		return cue, err
	}
	end, err := parseTimestampParts(m[5], m[6], m[7], m[8])
	if err != nil {
		return cue, err
	}
	if end <= start {
		return cue, fmt.Errorf("srt: cue %d ends (%s) before or at its start (%s)",
			autoSeq, FormatTime(end), FormatTime(start))
	}
	cue.Start, cue.End = start, end
	cue.Position = parsePosition(m[9])
	cue.Lines = append([]string(nil), lines[idx+1:]...)
	return cue, nil
}

func parseTimestampParts(h, m, s, ms string) (time.Duration, error) {
	hour, err := strconv.Atoi(h)
	if err != nil {
		return 0, err
	}
	min, err := strconv.Atoi(m)
	if err != nil {
		return 0, err
	}
	sec, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	milli, err := strconv.Atoi(ms)
	if err != nil {
		return 0, err
	}
	milli = padMillis(ms, milli)
	return time.Duration(hour)*time.Hour +
		time.Duration(min)*time.Minute +
		time.Duration(sec)*time.Second +
		time.Duration(milli)*time.Millisecond, nil
}

// parsePosition derives a region from SRT pixel coordinates. Coordinates with
// Y1 above the middle of a nominal 480px canvas are treated as top placement.
func parsePosition(suffix string) Position {
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return Position{}
	}
	p := Position{Has: true, Raw: suffix}
	for _, m := range coordRe.FindAllStringSubmatch(suffix, -1) {
		v, _ := strconv.Atoi(m[2])
		switch m[1] {
		case "X1":
			p.X1 = v
		case "Y1":
			p.Y1 = v
		case "X2":
			p.X2 = v
		case "Y2":
			p.Y2 = v
		}
	}
	return p
}

// RegionFromPosition maps parsed coordinates to a vertical region.
func RegionFromPosition(p Position) Region {
	if !p.Has {
		return RegionUnspecified
	}
	if p.Y1 < 240 {
		return RegionTop
	}
	return RegionBottom
}

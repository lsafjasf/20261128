package subburn

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ParseSRT reads the common SubRip form. Cue times are interpreted on the
// source display clock; no frame-rate mapping is applied here.
func ParseSRT(r io.Reader) ([]Cue, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if len(lines) == 0 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var blocks [][]string
	var block []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if len(block) > 0 {
				blocks = append(blocks, block)
				block = nil
			}
			continue
		}
		block = append(block, line)
	}
	if len(block) > 0 {
		blocks = append(blocks, block)
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("SRT contains no cues")
	}

	cues := make([]Cue, 0, len(blocks))
	for index, block := range blocks {
		timeLine := -1
		for i, line := range block {
			if strings.Contains(line, "-->") {
				timeLine = i
				break
			}
		}
		if timeLine < 0 {
			return nil, fmt.Errorf("SRT block %d has no timing line", index+1)
		}
		start, end, err := parseSRTRange(block[timeLine])
		if err != nil {
			return nil, fmt.Errorf("SRT block %d: %w", index+1, err)
		}
		id := fmt.Sprintf("cue-%d", index+1)
		if timeLine > 0 {
			if candidate := strings.TrimSpace(block[timeLine-1]); candidate != "" {
				id = candidate
			}
		}
		text := strings.Join(block[timeLine+1:], "\n")
		cue := Cue{ID: id, Start: start, End: end, Text: text}
		if err := cue.Validate(); err != nil {
			return nil, err
		}
		cues = append(cues, cue)
	}
	return cues, nil
}

func parseSRTRange(line string) (time.Duration, time.Duration, error) {
	parts := strings.SplitN(line, "-->", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid timing line %q", line)
	}
	startText := strings.Fields(strings.TrimSpace(parts[0]))
	endText := strings.Fields(strings.TrimSpace(parts[1]))
	if len(startText) == 0 || len(endText) == 0 {
		return 0, 0, fmt.Errorf("invalid timing line %q", line)
	}
	start, err := parseSRTTime(startText[0])
	if err != nil {
		return 0, 0, err
	}
	end, err := parseSRTTime(endText[0])
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func parseSRTTime(value string) (time.Duration, error) {
	value = strings.Replace(value, ",", ".", 1)
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid SRT time %q", value)
	}
	hours, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid SRT time %q", value)
	}
	minutes, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || minutes < 0 || minutes > 59 {
		return 0, fmt.Errorf("invalid SRT time %q", value)
	}
	secondsPart := strings.SplitN(parts[2], ".", 2)
	seconds, err := strconv.ParseInt(secondsPart[0], 10, 64)
	if err != nil || seconds < 0 || seconds > 59 {
		return 0, fmt.Errorf("invalid SRT time %q", value)
	}
	var millis int64
	if len(secondsPart) == 2 {
		frac := secondsPart[1]
		if len(frac) > 3 {
			frac = frac[:3]
		}
		for len(frac) < 3 {
			frac += "0"
		}
		millis, err = strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid SRT time %q", value)
		}
	}
	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(millis)*time.Millisecond, nil
}

func FormatSRT(w io.Writer, cues []Cue) error {
	writer := bufio.NewWriter(w)
	for i, cue := range cues {
		if err := cue.Validate(); err != nil {
			return err
		}
		if i > 0 {
			if _, err := fmt.Fprintln(writer); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(writer, "%d\n%s --> %s\n", i+1,
			formatSRTTime(cue.Start), formatSRTTime(cue.End)); err != nil {
			return err
		}
		if cue.Text != "" {
			if _, err := fmt.Fprintln(writer, cue.Text); err != nil {
				return err
			}
		}
	}
	return writer.Flush()
}

func formatSRTTime(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	millis := value / time.Millisecond
	hours := millis / (60 * 60 * 1000)
	millis %= 60 * 60 * 1000
	minutes := millis / (60 * 1000)
	millis %= 60 * 1000
	seconds := millis / 1000
	millis %= 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

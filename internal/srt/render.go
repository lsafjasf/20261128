package srt

import (
	"strconv"
	"strings"
	"time"
)

// RenderCue is a cue ready to be written back out as SRT.
type RenderCue struct {
	Start  int64 // milliseconds
	End    int64
	Region Region
	Lines  []string
}

// Render writes cues in SubRip format, renumbering from 1.
// Top-region cues receive explicit Y coordinates so players keep them apart
// from bottom-region cues instead of drawing both on the same baseline.
func Render(cues []RenderCue) string {
	var b strings.Builder
	for i, cue := range cues {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString("\n")
		b.WriteString(FormatTime(ms(cue.Start)))
		b.WriteString(" --> ")
		b.WriteString(FormatTime(ms(cue.End)))
		if suffix, ok := regionSuffix(cue.Region); ok {
			b.WriteString(" ")
			b.WriteString(suffix)
		}
		b.WriteString("\n")
		b.WriteString(strings.Join(cue.Lines, "\n"))
		b.WriteString("\n")
	}
	return b.String()
}

func ms(v int64) (d time.Duration) {
	return time.Duration(v) * time.Millisecond
}

func regionSuffix(r Region) (string, bool) {
	switch r {
	case RegionTop:
		return "X1:0 Y1:20 X2:1280 Y2:120", true
	case RegionBottom:
		return "X1:0 Y1:360 X2:1280 Y2:460", true
	default:
		return "", false
	}
}

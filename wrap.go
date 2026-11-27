package subtitlelayout

import (
	"fmt"
	"strings"
)

const widthTolerance = 1e-9

func (l *Layouter) wrap(clusters []cluster, logicalLine int) ([]Line, error) {
	if len(clusters) == 0 {
		return []Line{{}}, nil
	}

	lines := make([]Line, 0, 1)
	start := 0
	for start < len(clusters) {
		width := 0.0
		end := start
		lastBreak := -1
		lastBreakWidth := 0.0

		for end < len(clusters) {
			nextWidth := width + clusters[end].width
			if nextWidth > l.contentWidth+widthTolerance {
				break
			}
			width = nextWidth
			end++
			if end < len(clusters) && canBreakBetween(clusters[end-1], clusters[end]) {
				lastBreak = end
				lastBreakWidth = width
			}
		}

		if end == len(clusters) {
			lines = append(lines, makeLine(clusters, start, end, width))
			break
		}
		if lastBreak > start {
			lines = append(lines, makeLine(clusters, start, lastBreak, lastBreakWidth))
			start = lastBreak
			continue
		}

		offending := clusters[start]
		if end > start {
			offending = clusters[end]
		}
		return nil, fmt.Errorf(
			"%w: logical line %d, glyph U+%04X at byte %d; available width %v, line width would exceed it",
			ErrTextDoesNotFit, logicalLine, offending.lead, offending.start, l.contentWidth,
		)
	}
	return lines, nil
}

func makeLine(clusters []cluster, start, end int, width float64) Line {
	var text strings.Builder
	for i := start; i < end; i++ {
		text.WriteString(clusters[i].text)
	}
	return Line{Text: text.String(), Width: width}
}

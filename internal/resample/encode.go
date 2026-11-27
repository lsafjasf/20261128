package resample

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func f6(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

func joinInts(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, ";")
}

func joinFloats(xs []float64) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = f6(x)
	}
	return strings.Join(parts, ";")
}

// WriteFramesCSV emits one row per output frame, matching the analysis
// spreadsheet schema.
func WriteFramesCSV(w io.Writer, rep Report) error {
	cw := csv.NewWriter(w)
	header := []string{
		"output_index", "pts_ms", "kind", "duplicate", "synthesized",
		"source_indices", "alpha", "target_pts_ms", "effective_pts_ms",
		"phase_error_ms", "missing_source", "actual_step_ms", "ideal_step_ms",
		"signed_error_ms", "normalized_error", "magnitude", "class",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for i, f := range rep.Frames {
		p := rep.JudderCurve[i]
		row := []string{
			strconv.Itoa(f.OutputIndex),
			f6(f.PtsMs),
			f.Kind,
			strconv.FormatBool(f.Duplicate),
			strconv.FormatBool(f.Synthesized),
			joinInts(f.SourceIndices),
			f6(f.Alpha),
			f6(f.TargetPtsMs),
			f6(f.EffectivePtsMs),
			f6(f.PhaseErrorMs),
			strconv.FormatBool(f.MissingSource),
			f6(p.ActualStepMs),
			f6(p.IdealStepMs),
			f6(p.SignedErrorMs),
			f6(p.NormalizedError),
			f6(p.Magnitude),
			p.Class,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteJSON emits the report or comparison as indented JSON.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return nil
}

// WriteJudderSVG draws the normalized judder curves of several reports on
// shared axes so pause (negative) and jump (positive) patterns are visible.
func WriteJudderSVG(w io.Writer, nameByReport map[string]Report) error {
	names := make([]string, 0, len(nameByReport))
	for name := range nameByReport {
		names = append(names, name)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}

	const (
		width  = 960
		height = 420
		margin = 48
	)
	maxN := 0
	peak := 1.0
	for _, name := range names {
		rep := nameByReport[name]
		if n := len(rep.JudderCurve); n > maxN {
			maxN = n
		}
		for _, p := range rep.JudderCurve {
			if m := abs64(p.NormalizedError); m > peak {
				peak = m
			}
		}
	}
	plotW := float64(width - 2*margin)
	plotH := float64(height - 2*margin)
	x := func(i int) float64 {
		return margin + float64(i)*plotW/float64(maxN-1)
	}
	y := func(v float64) float64 {
		return margin + plotH*(1-(v+peak)/(2*peak))
	}

	colors := map[string]string{
		"before": "#d1495b",
		"after":  "#2e8b57",
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\">\n",
		width, height, width, height)
	b.WriteString("<rect width=\"100%\" height=\"100%\" fill=\"white\"/>\n")
	// Zero line and gridlines.
	fmt.Fprintf(&b, "<line x1=\"%d\" y1=\"%.2f\" x2=\"%d\" y2=\"%.2f\" stroke=\"#888\" stroke-width=\"1\"/>\n",
		margin, y(0), width-margin, y(0))
	for _, gv := range []float64{-peak, -peak / 2, peak / 2, peak} {
		fmt.Fprintf(&b, "<line x1=\"%d\" y1=\"%.2f\" x2=\"%d\" y2=\"%.2f\" stroke=\"#eee\"/>\n",
			margin, y(gv), width-margin, y(gv))
		fmt.Fprintf(&b, "<text x=\"%d\" y=\"%.2f\" font-size=\"10\" fill=\"#666\">%.2f</text>\n",
			6, y(gv)+3, gv)
	}
	fmt.Fprintf(&b, "<text x=\"%d\" y=\"%d\" font-size=\"12\" fill=\"#333\">output frame index</text>\n",
		width/2-40, height-12)
	fmt.Fprintf(&b, "<text x=\"12\" y=\"16\" font-size=\"12\" fill=\"#333\">normalized step error</text>\n")

	for _, name := range names {
		rep := nameByReport[name]
		color, ok := colors[name]
		if !ok {
			color = "#3366cc"
		}
		var pts []string
		for i, p := range rep.JudderCurve {
			pts = append(pts, fmt.Sprintf("%.2f,%.2f", x(i), y(p.NormalizedError)))
		}
		fmt.Fprintf(&b, "<polyline fill=\"none\" stroke=\"%s\" stroke-width=\"1.4\" points=\"%s\"/>\n",
			color, strings.Join(pts, " "))
	}

	legendX := width - margin - 150
	for i, name := range names {
		color := colors[name]
		if color == "" {
			color = "#3366cc"
		}
		ly := margin + 14 + i*18
		fmt.Fprintf(&b, "<rect x=\"%d\" y=\"%d\" width=\"12\" height=\"12\" fill=\"%s\"/>\n",
			legendX, ly, color)
		fmt.Fprintf(&b, "<text x=\"%d\" y=\"%d\" font-size=\"12\" fill=\"#333\">%s (rms %.4f)</text>\n",
			legendX+16, ly+10, name, nameByReport[name].Summary.RMSNormalizedJudder)
	}
	b.WriteString("</svg>\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

package ass

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// WebVTT renders all dialogue events as static cues.
func (script *Script) WebVTT(options Options) (string, error) {
	cues, err := script.Segments(options)
	if err != nil {
		return "", err
	}
	return renderWebVTT(cues, script.Style), nil
}

func renderWebVTT(cues []Cue, style Style) string {
	var out strings.Builder
	out.WriteString("WEBVTT\n\n")

	hasStyles := false
	for index := range cues {
		if cueHasCSS(&cues[index], style) {
			if !hasStyles {
				out.WriteString("STYLE\n")
				hasStyles = true
			}
			fmt.Fprintf(&out, "::cue(.ass-%d) {\n", index)
			writeCueCSS(&out, cues[index], style)
			out.WriteString("}\n")
		}
	}
	if hasStyles {
		out.WriteByte('\n')
	}

	for index := range cues {
		cue := &cues[index]
		fmt.Fprintf(&out, "cue-%d\n", index+1)
		fmt.Fprintf(&out, "%s --> %s", FormatTimestamp(cue.Start), FormatTimestamp(cue.End))
		if settings := cueSettings(*cue, style); settings != "" {
			out.WriteByte(' ')
			out.WriteString(settings)
		}
		out.WriteByte('\n')
		text := vttEscape(cue.Text)
		if cueHasCSS(cue, style) {
			fmt.Fprintf(&out, "<c.ass-%d>%s</c>", index, text)
		} else {
			out.WriteString(text)
		}
		out.WriteString("\n\n")
	}
	return strings.TrimRight(out.String(), "\n") + "\n"
}

func cueHasCSS(cue *Cue, style Style) bool {
	state := cue.State
	return state.ScaleX != 100 ||
		state.ScaleY != 100 ||
		state.RotateX != 0 ||
		state.RotateY != 0 ||
		state.RotateZ != 0 ||
		state.Origin != nil
}

func writeCueCSS(out *strings.Builder, cue Cue, style Style) {
	state := cue.State
	out.WriteString("  display: inline-block;\n")
	if state.Origin != nil {
		x := percent(state.Origin.X, float64(style.PlayResX))
		y := percent(state.Origin.Y, float64(style.PlayResY))
		fmt.Fprintf(out, "  transform-origin: %s%% %s%%;\n", number(x), number(y))
	}
	var transforms []string
	if state.RotateX != 0 {
		transforms = append(transforms, "rotateX("+number(state.RotateX)+"deg)")
	}
	if state.RotateY != 0 {
		transforms = append(transforms, "rotateY("+number(state.RotateY)+"deg)")
	}
	if state.RotateZ != 0 {
		transforms = append(transforms, "rotateZ("+number(state.RotateZ)+"deg)")
	}
	if state.ScaleX != 100 || state.ScaleY != 100 {
		transforms = append(transforms, "scale("+number(state.ScaleX/100)+", "+number(state.ScaleY/100)+")")
	}
	if len(transforms) > 0 {
		fmt.Fprintf(out, "  transform: %s;\n", strings.Join(transforms, " "))
	}
	out.WriteString("  white-space: pre-wrap;\n")
}

func cueSettings(cue Cue, style Style) string {
	if cue.State.Position == nil {
		return ""
	}
	x := percent(cue.State.Position.X, float64(style.PlayResX))
	y := percent(cue.State.Position.Y, float64(style.PlayResY))
	align := cueAlign(cue.State.Alignment)
	return fmt.Sprintf("position:%s%% line:%s%% align:%s",
		number(x), number(y), align)
}

func cueAlign(alignment int) string {
	switch alignment {
	case 1, 4, 7:
		return "start"
	case 3, 6, 9:
		return "end"
	default:
		return "center"
	}
}

func percent(value, scale float64) float64 {
	if scale <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	result := value / scale * 100
	if result < 0 {
		return 0
	}
	if result > 100 {
		return 100
	}
	return result
}

func number(value float64) string {
	if math.Abs(value) < 0.0005 {
		return "0"
	}
	return strconv.FormatFloat(value, 'f', 3, 64)
}

func vttEscape(text string) string {
	var out strings.Builder
	for _, r := range text {
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// Convert reads ASS from r and returns a WebVTT document.
func Convert(r io.Reader, options Options) (string, error) {
	script, err := Parse(r)
	if err != nil {
		return "", err
	}
	return script.WebVTT(options)
}

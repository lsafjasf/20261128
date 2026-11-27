package layout

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type computedStyle struct {
	fontSize     float64
	lineHeight   float64
	textAlign    string
	displayAlign string
	rubyPosition string
	rubyAlign    string
	padding      [4]float64
}

func computeStyle(props map[string]string, parentFont, viewportW, viewportH float64) computedStyle {
	style := computedStyle{
		fontSize:     32,
		textAlign:    "start",
		displayAlign: "center",
		rubyPosition: "before",
		rubyAlign:    "center",
	}
	if parentFont > 0 {
		style.fontSize = parentFont
	}

	if raw := props["fontSize"]; raw != "" {
		if value, ok := parseLength(raw, parentFont, parentFont, viewportW, viewportH); ok && value > 0 {
			style.fontSize = value
		}
	}
	style.lineHeight = style.fontSize * 1.2
	if raw := props["lineHeight"]; raw != "" && !strings.EqualFold(raw, "normal") {
		if value, ok := parseLength(raw, style.fontSize, style.fontSize, viewportW, viewportH); ok && value > 0 {
			style.lineHeight = value
		}
	}
	if value := props["textAlign"]; value != "" {
		style.textAlign = value
	}
	if value := props["displayAlign"]; value != "" {
		style.displayAlign = value
	}
	if value := props["rubyPosition"]; value != "" {
		style.rubyPosition = value
	}
	if value := props["rubyAlign"]; value != "" {
		style.rubyAlign = value
	}
	style.padding = parsePadding(props["padding"], style.fontSize, viewportW, viewportH)
	return style
}

func parsePadding(raw string, font, viewportW, viewportH float64) [4]float64 {
	var result [4]float64
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return result
	}
	values := make([]float64, 0, len(fields))
	for _, field := range fields {
		value, ok := parseLength(field, font, font, viewportW, viewportH)
		if !ok {
			return result
		}
		values = append(values, value)
	}
	switch len(values) {
	case 1:
		result = [4]float64{values[0], values[0], values[0], values[0]}
	case 2:
		result = [4]float64{values[0], values[1], values[0], values[1]}
	case 3:
		result = [4]float64{values[0], values[1], values[2], values[1]}
	default:
		result = [4]float64{values[0], values[1], values[2], values[3]}
	}
	return result
}

func parseLength(raw string, percentReference, emReference, viewportW, viewportH float64) (float64, bool) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return 0, false
	}
	units := []struct {
		suffix string
		scale  func(float64) float64
	}{
		{"rem", func(v float64) float64 { return v * emReference }},
		{"px", func(v float64) float64 { return v }},
		{"%", func(v float64) float64 { return v * percentReference / 100 }},
		{"em", func(v float64) float64 { return v * emReference }},
		{"rw", func(v float64) float64 { return v * viewportW / 100 }},
		{"rh", func(v float64) float64 { return v * viewportH / 100 }},
		{"c", func(v float64) float64 { return v * emReference }},
	}
	for _, unit := range units {
		if !strings.HasSuffix(value, unit.suffix) {
			continue
		}
		number := strings.TrimSpace(strings.TrimSuffix(value, unit.suffix))
		parsed, err := strconv.ParseFloat(number, 64)
		if err != nil {
			continue
		}
		return unit.scale(parsed), true
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func parsePair(raw string, xPercentRef, yPercentRef, emReference, viewportW, viewportH float64) (float64, float64, bool) {
	fields := strings.Fields(raw)
	if len(fields) != 2 {
		return 0, 0, false
	}
	x, okX := parseLength(fields[0], xPercentRef, emReference, viewportW, viewportH)
	y, okY := parseLength(fields[1], yPercentRef, emReference, viewportW, viewportH)
	return x, y, okX && okY
}

func rectFromTTML(origin, extent string, viewportW, viewportH, font float64) Rect {
	var rect Rect
	if x, y, ok := parsePair(origin, viewportW, viewportH, font, viewportW, viewportH); ok {
		rect.X, rect.Y = x, y
	}
	if width, height, ok := parsePair(extent, viewportW, viewportH, font, viewportW, viewportH); ok {
		rect.Width, rect.Height = width, height
	}
	return rect
}

func finite(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func (s computedStyle) String() string {
	return fmt.Sprintf("font=%.2f line=%.2f", s.fontSize, s.lineHeight)
}

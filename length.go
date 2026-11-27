// Package imscfit parses IMSC/TTML documents and rebuilds the effective line
// width of subtitle regions across canvas aspect ratios, using only the Go
// standard library.
package imscfit

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Unit identifies the unit of a TTML length value.
type Unit int

const (
	UnitPX      Unit = iota // tts:extent style pixels
	UnitCell                // "c", relative to the root container cell grid
	UnitEM                  // "em", relative to the current font size
	UnitPercent             // "%", relative to a reference dimension
)

// Length is a parsed TTML length value.
type Length struct {
	Value float64 `json:"value"`
	Unit  Unit    `json:"unit"`
	// Set is false when the attribute was absent.
	Set bool `json:"set"`
}

func (l Length) String() string {
	if !l.Set {
		return "<unset>"
	}
	switch l.Unit {
	case UnitPX:
		return fmt.Sprintf("%gpx", l.Value)
	case UnitCell:
		return fmt.Sprintf("%gc", l.Value)
	case UnitEM:
		return fmt.Sprintf("%gem", l.Value)
	case UnitPercent:
		return fmt.Sprintf("%g%%", l.Value)
	default:
		return strconv.FormatFloat(l.Value, 'g', -1, 64)
	}
}

// MarshalJSON renders the unit symbol instead of its numeric enum.
func (u Unit) MarshalJSON() ([]byte, error) {
	switch u {
	case UnitPX:
		return json.Marshal("px")
	case UnitCell:
		return json.Marshal("c")
	case UnitEM:
		return json.Marshal("em")
	case UnitPercent:
		return json.Marshal("%")
	default:
		return json.Marshal("unknown")
	}
}

// ParseLength parses values such as "80%", "12c", "1.5em", "1920px".
func ParseLength(s string) (Length, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Length{}, nil
	}
	i := strings.IndexAny(s, "pcem%")
	if i < 0 {
		// Unit-less TTML lengths default to the default font size (1em).
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return Length{}, fmt.Errorf("invalid length %q", s)
		}
		return Length{Value: v, Unit: UnitEM, Set: true}, nil
	}
	num, unit := s[:i], s[i:]
	v, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
	if err != nil {
		return Length{}, fmt.Errorf("invalid length %q: %w", s, err)
	}
	l := Length{Value: v, Set: true}
	switch unit {
	case "px":
		l.Unit = UnitPX
	case "c":
		l.Unit = UnitCell
	case "em":
		l.Unit = UnitEM
	case "%":
		l.Unit = UnitPercent
	default:
		return Length{}, fmt.Errorf("unsupported length unit %q", unit)
	}
	return l, nil
}

// Ref carries the reference dimensions needed to resolve a length.
type Ref struct {
	CanvasW, CanvasH float64 // root container extent in pixels
	CellW, CellH     float64 // size of one grid cell in pixels
	FontSize         float64 // current font size in pixels
	RefW, RefH       float64 // percentage reference box in pixels
}

// Resolve converts the length to pixels. horizontal selects the reference
// dimension used for percentage and cell units.
func (l Length) Resolve(r Ref, horizontal bool) float64 {
	if !l.Set {
		return 0
	}
	switch l.Unit {
	case UnitPX:
		return l.Value
	case UnitCell:
		if horizontal {
			return l.Value * r.CellW
		}
		return l.Value * r.CellH
	case UnitEM:
		return l.Value * r.FontSize
	case UnitPercent:
		if horizontal {
			return l.Value / 100 * r.RefW
		}
		return l.Value / 100 * r.RefH
	default:
		return 0
	}
}

// RoundPx rounds a pixel measure to a whole number, keeping a small epsilon
// out of the result.
func RoundPx(v float64) float64 {
	return math.Round(v*100) / 100
}

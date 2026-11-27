// Package ass parses Advanced SubStation Alpha (ASS) dialogue and converts
// timed override animations into static WebVTT cues.
package ass

import "time"

// DefaultPlayRes is used when Script Info does not declare a resolution.
const (
	DefaultPlayResX = 384
	DefaultPlayResY = 288
)

// Point is a position in ASS script coordinates.
type Point struct {
	X float64
	Y float64
}

// Transform is the visual state that can be carried by a static cue segment.
// Scale values are ASS percentages: 100 means the original size.
type Transform struct {
	ScaleX    float64
	ScaleY    float64
	RotateX   float64
	RotateY   float64
	RotateZ   float64
	Position  *Point
	Origin    *Point
	Alignment int
}

// DefaultTransform returns the ASS default: 100% scale, no rotation, and
// baseline bottom-centre alignment. Position is nil until \pos or \move sets it.
func DefaultTransform() Transform {
	return Transform{
		ScaleX:    100,
		ScaleY:    100,
		Alignment: 2,
	}
}

// Clone copies a transform without sharing Point values.
func (t Transform) Clone() Transform {
	out := t
	if t.Position != nil {
		position := *t.Position
		out.Position = &position
	}
	if t.Origin != nil {
		origin := *t.Origin
		out.Origin = &origin
	}
	return out
}

// Equal reports whether two transforms have identical values.
func (t Transform) Equal(other Transform) bool {
	return t.ScaleX == other.ScaleX &&
		t.ScaleY == other.ScaleY &&
		t.RotateX == other.RotateX &&
		t.RotateY == other.RotateY &&
		t.RotateZ == other.RotateZ &&
		pointsEqual(t.Position, other.Position) &&
		pointsEqual(t.Origin, other.Origin) &&
		t.Alignment == other.Alignment
}

func pointsEqual(left, right *Point) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// Style contains the Script Info values needed to place static cues.
type Style struct {
	PlayResX int
	PlayResY int
}

// Event is one ASS Dialogue line.
type Event struct {
	Layer   int
	Style   string
	Name    string
	Start   time.Duration
	End     time.Duration
	MarginL int
	MarginR int
	MarginV int
	Effect  string
	Text    string
	Line    int
}

// Script is a parsed ASS document.
type Script struct {
	Info   map[string]string
	Format []string
	Events []Event
	Style  Style
}

// Segment is one static interval emitted from an ASS event.
type Segment struct {
	Start time.Duration
	End   time.Duration
	Text  string
	State Transform
}

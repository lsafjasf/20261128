package layout

import (
	"fmt"
	"math"
)

// Rect is an axis-aligned rectangle in TTML pixel coordinates.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (r Rect) Right() float64  { return r.X + r.Width }
func (r Rect) Bottom() float64 { return r.Y + r.Height }

func (r Rect) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

// Union returns the smallest rectangle containing both inputs. An empty
// rectangle is treated as neutral, which is useful when accumulating ink boxes.
func (r Rect) Union(other Rect) Rect {
	if r.Empty() {
		return other
	}
	if other.Empty() {
		return r
	}
	left := math.Min(r.X, other.X)
	top := math.Min(r.Y, other.Y)
	right := math.Max(r.Right(), other.Right())
	bottom := math.Max(r.Bottom(), other.Bottom())
	return Rect{X: left, Y: top, Width: right - left, Height: bottom - top}
}

// MetricFace supplies glyph advances and vertical metrics in em units. A
// false return from Advance means that the face has no glyph for the rune.
// Callers normally compose several faces into a fallback chain.
type MetricFace interface {
	Advance(r rune) (em float64, ok bool)
	Ascent() float64
	Descent() float64
}

// FaceChain is a font fallback chain. The first face that covers a rune wins.
type FaceChain []MetricFace

func (c FaceChain) Advance(r rune) (float64, bool) {
	for _, face := range c {
		if face == nil {
			continue
		}
		if em, ok := face.Advance(r); ok {
			return em, true
		}
	}
	return 0, false
}

func (c FaceChain) Ascent() float64 {
	for _, face := range c {
		if face != nil {
			return face.Ascent()
		}
	}
	return 0.88
}

func (c FaceChain) Descent() float64 {
	for _, face := range c {
		if face != nil {
			return face.Descent()
		}
	}
	return 0.12
}

// Options controls parsing and layout. Zero values select the defaults.
type Options struct {
	ViewportWidth  float64
	ViewportHeight float64
	RegionID       string
	Paragraph      int
	Face           MetricFace
	FallbackRune   rune
	LeftPadding    float64
	RightPadding   float64
	TopPadding     float64
	BottomPadding  float64
	RubyGap        float64
	RubyScale      float64
}

func (o Options) withDefaults() Options {
	if o.Face == nil {
		o.Face = FaceChain{SyntheticFace{}}
	}
	if o.FallbackRune == 0 {
		o.FallbackRune = '\uFFFD'
	}
	if o.RubyScale <= 0 {
		o.RubyScale = 0.5
	}
	if o.RubyGap < 0 {
		o.RubyGap = 0
	}
	return o
}

// SyntheticFace is a deterministic metric-only face used when no font file is
// supplied. It covers printable runes and gives CJK ideographs full-em widths.
type SyntheticFace struct{}

func (SyntheticFace) Advance(r rune) (float64, bool) {
	if r == '\uFFFD' || r == '\u25A1' {
		return 1, true
	}
	if r < 0x20 || (r >= 0x7f && r < 0xa0) {
		return 0, false
	}
	if isWideRune(r) {
		return 1, true
	}
	if r == ' ' {
		return 0.25, true
	}
	return 0.55, true
}

func (SyntheticFace) Ascent() float64  { return 0.88 }
func (SyntheticFace) Descent() float64 { return 0.12 }

// MissingGlyph records a rune that had to use a replacement glyph.
type MissingGlyph struct {
	Rune    string  `json:"rune"`
	Code    string  `json:"codePoint"`
	Layer   string  `json:"layer"`
	Line    int     `json:"line"`
	X       float64 `json:"x"`
	Message string  `json:"message"`
}

// RubyLine is one annotation layer box. Layer 0 is closest to the base text.
type RubyLine struct {
	Layer    int            `json:"layer"`
	Text     string         `json:"text"`
	Box      Rect           `json:"box"`
	Baseline float64        `json:"baseline"`
	Missing  []MissingGlyph `json:"missing,omitempty"`
}

// Line is one logical base-text line. Box and Baseline are intentionally
// independent from the ruby layer boxes: adding ruby must not change the base
// line pitch or baseline.
type Line struct {
	Text     string         `json:"text"`
	Box      Rect           `json:"box"`
	Baseline float64        `json:"baseline"`
	Ruby     []RubyLine     `json:"ruby,omitempty"`
	Missing  []MissingGlyph `json:"missing,omitempty"`
}

// Result is the JSON-facing result of laying out one subtitle paragraph.
type Result struct {
	Region        Rect           `json:"region"`
	ContentBounds Rect           `json:"contentBounds"`
	Lines         []Line         `json:"lines"`
	Missing       []MissingGlyph `json:"missing,omitempty"`
}

func (r Result) String() string {
	return fmt.Sprintf("region=(%.2f,%.2f %.2fx%.2f) lines=%d",
		r.Region.X, r.Region.Y, r.Region.Width, r.Region.Height, len(r.Lines))
}

package imscfit

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Geometry is the resolved pixel geometry of a region on one canvas.
type Geometry struct {
	OriginX   float64 `json:"originX"`
	OriginY   float64 `json:"originY"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	PadTop    float64 `json:"padTop"`
	PadRight  float64 `json:"padRight"`
	PadBottom float64 `json:"padBottom"`
	PadLeft   float64 `json:"padLeft"`
	ContentW  float64 `json:"contentWidth"`
	ContentH  float64 `json:"contentHeight"`
	OffCanvas bool    `json:"offCanvas"`
}

// Canvas describes a target rendering surface in pixels.
type Canvas struct {
	Name   string  `json:"name"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (d *Document) cellSize(c Canvas) (cw, ch float64) {
	ch = c.Height / float64(d.CellY)
	cw = c.Width / float64(d.CellX)
	return cw, ch
}

func (d *Document) defaultFontPx(c Canvas) float64 {
	_, ch := d.cellSize(c)
	return ch
}

// resolveFont converts a font size length to pixels inside the given canvas.
func (d *Document) resolveFont(l Length, parentPx float64, c Canvas) float64 {
	_, ch := d.cellSize(c)
	base := d.defaultFontPx(c)
	switch {
	case !l.Set:
		if parentPx > 0 {
			return parentPx
		}
		return base
	case l.Unit == UnitCell:
		return l.Value * ch
	case l.Unit == UnitEM:
		ref := parentPx
		if ref == 0 {
			ref = base
		}
		return l.Value * ref
	case l.Unit == UnitPX:
		return l.Value
	case l.Unit == UnitPercent:
		ref := parentPx
		if ref == 0 {
			ref = base
		}
		return l.Value / 100 * ref
	default:
		return base
	}
}

// ResolveGeometry computes a region box and its padded content box for a
// canvas. Missing origin/extent fall back to the whole canvas.
func (d *Document) ResolveGeometry(r *Region, props Props, c Canvas) Geometry {
	cw, ch := d.cellSize(c)
	fontPx := d.resolveFont(props.FontSize, 0, c)

	ref := Ref{CanvasW: c.Width, CanvasH: c.Height, CellW: cw, CellH: ch,
		FontSize: fontPx, RefW: c.Width, RefH: c.Height}

	g := Geometry{}
	if r.Extent.X.Set {
		g.Width = r.Extent.X.Resolve(ref, true)
	} else {
		g.Width = c.Width
	}
	if r.Extent.Y.Set {
		g.Height = r.Extent.Y.Resolve(ref, false)
	} else {
		g.Height = c.Height
	}
	if r.Origin.X.Set {
		g.OriginX = r.Origin.X.Resolve(ref, true)
	}
	if r.Origin.Y.Set {
		g.OriginY = r.Origin.Y.Resolve(ref, false)
	}

	regionRef := Ref{CellW: cw, CellH: ch, FontSize: fontPx, RefW: g.Width, RefH: g.Height}
	g.PadLeft = props.Padding.Left.Resolve(regionRef, true)
	g.PadRight = props.Padding.Right.Resolve(regionRef, true)
	g.PadTop = props.Padding.Top.Resolve(regionRef, false)
	g.PadBottom = props.Padding.Bottom.Resolve(regionRef, false)

	g.ContentW = math.Max(0, g.Width-g.PadLeft-g.PadRight)
	g.ContentH = math.Max(0, g.Height-g.PadTop-g.PadBottom)
	g.OffCanvas = g.OriginX < 0 || g.OriginY < 0 ||
		g.OriginX+g.Width > c.Width+0.5 || g.OriginY+g.Height > c.Height+0.5
	return g
}

// atom is a piece of text tagged with its resolved font size.
type atom struct {
	runes  []rune
	fontPx float64
	space  bool
	breakk bool
}

func (d *Document) buildAtoms(p *Paragraph, c Canvas) []atom {
	out := make([]atom, 0, len(p.Segments))
	parentPx := 0.0
	for _, seg := range p.Segments {
		if seg.Break {
			out = append(out, atom{breakk: true})
			continue
		}
		fontPx := d.resolveFont(seg.FontSize, parentPx, c)
		parentPx = fontPx
		for _, field := range strings.Fields(seg.Text) {
			out = append(out, atom{runes: []rune(field), fontPx: fontPx, space: true})
		}
	}
	return out
}

func charWidthEm(r rune) float64 {
	switch {
	case r < 0x20:
		return 0
	case r == utf8.RuneError:
		return 1
	case r > 0x2E7F && !unicode.Is(unicode.Mn, r):
		return 1 // CJK, fullwidth forms, CJK punctuation
	case r == 0x2018 || r == 0x2019 || r == 0x201C || r == 0x201D:
		return 0.5
	case r >= '0' && r <= '9', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		return 0.55
	case r == ' ' || r == '\t':
		return 0.28
	case r >= 0x2000 && r <= 0x200A:
		return 0.5
	case r > 0x7F:
		return 0.6
	default:
		return 0.5
	}
}

func textWidthPx(s string, fontPx float64) float64 {
	w := 0.0
	for _, r := range s {
		w += charWidthEm(r) * fontPx
	}
	return w
}

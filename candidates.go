package imscfit

import (
	"fmt"
	"strings"
)

// TextCandidate is a shortened-text option that fits the region.
type TextCandidate struct {
	Text         string  `json:"text"`
	LineCount    int     `json:"lineCount"`
	UsedWidth    float64 `json:"usedWidth"`
	UsedHeight   float64 `json:"usedHeight"`
	RemovedRunes int     `json:"removedRunes"`
}

// RegionCandidate is a resized/relocated region option that fits the cue.
type RegionCandidate struct {
	Origin     string  `json:"origin"`
	Extent     string  `json:"extent"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	FitsCanvas bool    `json:"fitsCanvas"`
	Note       string  `json:"note,omitempty"`
}

// Candidates collects the remediation options for a failing cue.
type Candidates struct {
	RegionID      string            `json:"regionId"`
	ParagraphID   string            `json:"paragraphId"`
	ShortenText   []TextCandidate   `json:"shortenText,omitempty"`
	EnlargeRegion []RegionCandidate `json:"enlargeRegion,omitempty"`
}

// BuildCandidates produces concrete shorten/enlarge options for a cue whose
// report is not FitOK.
func (d *Document) BuildCandidates(p *Paragraph, c Canvas) Candidates {
	out := Candidates{RegionID: p.Region, ParagraphID: p.ID}
	r, ok := d.Regions[p.Region]
	if !ok {
		return out
	}
	props := d.EffectiveProps(r)
	g := d.ResolveGeometry(r, props, c)
	block := d.LayOut(p, props, g, c)
	if block.OverflowX <= 0.5 && block.OverflowY <= 0.5 {
		return out
	}

	out.ShortenText = d.shortenCandidates(p, props, g, c, block)
	if rc := d.regionCandidates(r, props, g, c, block); rc.Extent != "" {
		out.EnlargeRegion = append(out.EnlargeRegion, rc)
	}
	return out
}

func (d *Document) shortenCandidates(p *Paragraph, props Props, g Geometry, c Canvas, b Block) []TextCandidate {
	baseFont := d.resolveFont(props.FontSize, 0, c)
	raw := strings.TrimSpace(plainText(p))
	runes := []rune(strings.ReplaceAll(raw, "\n", " "))

	lineHeight := b.Lines[0].Height
	if lineHeight <= 0 {
		lineHeight = d.resolveLineHeight(props, baseFont)
	}

	noWrap := props.WrapOption == "noWrap"
	var best []textLine
	fallback := []rune("…")
	for n := len(runes) - 1; n > 0; n-- {
		candidate := strings.TrimRight(string(runes[:n]), " ,.;:、，。；：") + "…"
		var lines []textLine
		if noWrap {
			lines = joinNoWrap([]atom{{runes: []rune(candidate), fontPx: baseFont}})
		} else {
			lines = wrapGreedy([]atom{{runes: []rune(candidate), fontPx: baseFont}}, g.ContentW)
		}
		width, height := 0.0, float64(len(lines))*lineHeight
		for _, l := range lines {
			width = max64(width, textWidthPx(l.text, baseFont))
		}
		if width <= g.ContentW+0.5 && height <= g.ContentH+0.5 {
			best = lines
			fallback = []rune(candidate)
			break
		}
	}
	if best == nil {
		// Even the minimal "…" does not fit on one line; emit it anyway so
		// the operator still receives a concrete shortened-text option.
		best = joinNoWrap([]atom{{runes: fallback, fontPx: baseFont}})
	}
	width := 0.0
	for _, l := range best {
		width = max64(width, textWidthPx(l.text, baseFont))
	}
	return []TextCandidate{{
		Text:         string(fallback),
		LineCount:    len(best),
		UsedWidth:    RoundPx(width),
		UsedHeight:   RoundPx(float64(len(best)) * lineHeight),
		RemovedRunes: len([]rune(raw)) - len(fallback) + 1,
	}}
}

func (d *Document) regionCandidates(r *Region, props Props, g Geometry, c Canvas, b Block) RegionCandidate {
	needW := g.PadLeft + b.UsedW + g.PadRight
	needH := g.PadTop + b.UsedH + g.PadBottom

	width := clamp(needW, g.Width, c.Width)
	height := clamp(needH, g.Height, c.Height)

	originX, originY := g.OriginX, g.OriginY
	align := props.DisplayAlign
	switch align {
	case "after":
		originY = g.OriginY + g.Height - height
	case "center":
		originY = g.OriginY + (g.Height-height)/2
	}
	originX = clamp(originX, 0, mathMax(0, c.Width-width))
	originY = clamp(originY, 0, mathMax(0, c.Height-height))

	fits := needW <= c.Width+0.5 && needH <= c.Height+0.5
	note := ""
	if !fits {
		note = fmt.Sprintf("required %gx%g exceeds canvas %gx%g; shorten text instead",
			RoundPx(needW), RoundPx(needH), c.Width, c.Height)
	}
	return RegionCandidate{
		Origin:     fmt.Sprintf("%gpx %gpx", RoundPx(originX), RoundPx(originY)),
		Extent:     fmt.Sprintf("%gpx %gpx", RoundPx(width), RoundPx(height)),
		Width:      RoundPx(width),
		Height:     RoundPx(height),
		FitsCanvas: fits,
		Note:       note,
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

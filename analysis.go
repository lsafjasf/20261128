package imscfit

import "math"

// defaultRegionID is the synthetic region used for cues that reference no
// region; TTML maps those to the whole root container.
const defaultRegionID = "#default"

// Fit status of a cue against its region.
const (
	FitOK       = "ok"       // fits within the padded content box
	FitClipped  = "clipped"  // overflow=hidden hides part of the text
	FitSpilling = "spilling" // overflow=visible lets text leave the region
	FitNoRegion = "no-region"
)

// CueReport is the per-canvas analysis of one paragraph.
type CueReport struct {
	ParagraphID  string  `json:"paragraphId"`
	RegionID     string  `json:"regionId"`
	Begin        string  `json:"begin"`
	End          string  `json:"end"`
	Text         string  `json:"text"`
	Status       string  `json:"status"`
	LineCount    int     `json:"lineCount"`
	UsedWidth    float64 `json:"usedWidth"`
	UsedHeight   float64 `json:"usedHeight"`
	ContentW     float64 `json:"contentWidth"`
	ContentH     float64 `json:"contentHeight"`
	OverflowX    float64 `json:"overflowX"`
	OverflowY    float64 `json:"overflowY"`
	SpillTop     float64 `json:"spillTop"`
	SpillBottom  float64 `json:"spillBottom"`
	ClippedLines int     `json:"clippedLines"`
	Lines        []Line  `json:"lines"`
}

// RegionReport summarizes a region geometry on one canvas.
type RegionReport struct {
	ID       string   `json:"id"`
	Geometry Geometry `json:"geometry"`
	Props    Props    `json:"props"`
}

// CanvasReport holds the whole-document analysis for one canvas size.
type CanvasReport struct {
	Canvas  Canvas         `json:"canvas"`
	Regions []RegionReport `json:"regions"`
	Cues    []CueReport    `json:"cues"`
}

// Analyze resolves every region and paragraph against a canvas.
func (d *Document) Analyze(c Canvas) CanvasReport {
	rep := CanvasReport{Canvas: c}
	d.ensureDefaultRegion()

	for _, r := range d.Regions {
		props := d.EffectiveProps(r)
		g := d.ResolveGeometry(r, props, c)
		rep.Regions = append(rep.Regions, RegionReport{ID: r.ID, Geometry: g, Props: props})
	}

	for _, p := range d.Paragraphs {
		rep.Cues = append(rep.Cues, d.analyzeParagraph(p, c))
	}
	return rep
}

func (d *Document) analyzeParagraph(p *Paragraph, c Canvas) CueReport {
	rep := CueReport{
		ParagraphID: p.ID,
		RegionID:    orEmpty(p.Region),
		Begin:       p.Begin,
		End:         p.End,
		Text:        plainText(p),
	}
	r, ok := d.Regions[p.Region]
	if !ok {
		if p.Region != "" {
			rep.Status = FitNoRegion
			return rep
		}
		r = d.Regions[defaultRegionID]
	}
	props := d.EffectiveProps(r)
	g := d.ResolveGeometry(r, props, c)
	block := d.LayOut(p, props, g, c)

	rep.LineCount = len(block.Lines)
	rep.Lines = block.Lines
	rep.UsedWidth = RoundPx(block.UsedW)
	rep.UsedHeight = RoundPx(block.UsedH)
	rep.ContentW = RoundPx(g.ContentW)
	rep.ContentH = RoundPx(g.ContentH)
	rep.OverflowX = RoundPx(block.OverflowX)
	rep.OverflowY = RoundPx(block.OverflowY)

	align := props.DisplayAlign
	if align == "" {
		align = "before"
	}
	rep.SpillTop, rep.SpillBottom = spill(block, g, align)
	if props.Overflow == "hidden" {
		rep.ClippedLines = clippedLineCount(block, g, align)
	}

	switch {
	case block.OverflowX <= 0.5 && block.OverflowY <= 0.5:
		rep.Status = FitOK
	case props.Overflow == "hidden":
		rep.Status = FitClipped
	default:
		rep.Status = FitSpilling
	}
	return rep
}

func orEmpty(s string) string {
	if s == "" {
		return defaultRegionID
	}
	return s
}

func (d *Document) ensureDefaultRegion() {
	if _, ok := d.Regions[defaultRegionID]; ok {
		return
	}
	for _, p := range d.Paragraphs {
		if p.Region == "" {
			d.Regions[defaultRegionID] = &Region{ID: defaultRegionID}
			return
		}
	}
}

func spill(b Block, g Geometry, align string) (top, bottom float64) {
	if b.OverflowY <= 0.5 {
		return 0, 0
	}
	switch align {
	case "after":
		return RoundPx(b.OverflowY), 0
	case "center":
		half := b.OverflowY / 2
		return RoundPx(half), RoundPx(half)
	default: // before
		return 0, RoundPx(b.OverflowY)
	}
}

func clippedLineCount(b Block, g Geometry, align string) int {
	if b.OverflowY <= 0.5 || len(b.Lines) == 0 {
		return 0
	}
	lh := b.Lines[0].Height
	if lh <= 0 {
		return 0
	}
	visible := int(math.Floor((g.ContentH + 0.5) / lh))
	if visible >= len(b.Lines) {
		return 0
	}
	switch align {
	case "after":
		return len(b.Lines) - visible
	case "center":
		return int(math.Ceil(b.OverflowY / lh))
	default:
		return len(b.Lines) - visible
	}
}

func plainText(p *Paragraph) string {
	out := make([]rune, 0, 64)
	for _, s := range p.Segments {
		if s.Break {
			out = append(out, '\n')
			continue
		}
		out = append(out, []rune(s.Text)...)
	}
	return string(out)
}

package imscfit

// LengthPair is a horizontal/vertical length pair used by origin and extent.
type LengthPair struct {
	X, Y Length
}

// Padding holds up to four TTML padding lengths in CSS order:
// before(=top), end(=right), after(=bottom), start(=left).
type Padding struct {
	Top    Length `json:"top"`
	Right  Length `json:"right"`
	Bottom Length `json:"bottom"`
	Left   Length `json:"left"`
}

// AllZero reports whether no padding was specified.
func (p Padding) AllZero() bool {
	return !p.Top.Set && !p.Right.Set && !p.Bottom.Set && !p.Left.Set
}

// Props are style properties shared by styles and regions.
type Props struct {
	FontSize     Length  `json:"fontSize"`
	LineHeight   Length  `json:"lineHeight"`
	DisplayAlign string  `json:"displayAlign,omitempty"`
	Overflow     string  `json:"overflow,omitempty"`
	WrapOption   string  `json:"wrapOption,omitempty"`
	Padding      Padding `json:"padding"`
}

// Style is a named ttm:style element.
type Style struct {
	ID    string
	Refs  []string
	Props Props
}

// Region is a ttm:region definition.
type Region struct {
	ID     string
	Refs   []string
	Origin LengthPair
	Extent LengthPair
	Props
}

// Segment is a flattened piece of paragraph content: either literal text or a
// forced line break.
type Segment struct {
	Text     string
	Break    bool
	FontSize Length // effective font size at this point in the tree
}

// Paragraph is a <p> cue flattened into ordered segments.
type Paragraph struct {
	ID       string
	Region   string
	Begin    string
	End      string
	BeginSec float64
	EndSec   float64
	Timed    bool
	Segments []Segment
}

// Document is the parsed subset of an IMSC/TTML document needed for layout.
type Document struct {
	// CellResolution defaults to 32 x 15 per TTML.
	CellX int
	CellY int
	// PixelAspectRatio defaults to 1.
	PixelAspectRatio float64
	// RootExtent is tts:extent on <tt>; zero when absent.
	RootExtent LengthPair

	Styles     map[string]*Style
	Regions    map[string]*Region
	Paragraphs []*Paragraph
}

func newDocument() *Document {
	return &Document{
		CellX:            32,
		CellY:            15,
		PixelAspectRatio: 1,
		Styles:           map[string]*Style{},
		Regions:          map[string]*Region{},
	}
}

// EffectiveProps merges a region's referenced styles with its own attributes.
// The region's own attributes win; earlier style references win over later
// ones, matching TTML style resolution order.
func (d *Document) EffectiveProps(r *Region) Props {
	merged := Props{}
	for i := len(r.Refs) - 1; i >= 0; i-- {
		if s := d.Styles[r.Refs[i]]; s != nil {
			merged = mergeProps(merged, s.Props)
		}
	}
	return mergeProps(merged, r.Props)
}

func mergeProps(lower, higher Props) Props {
	out := lower
	if higher.FontSize.Set {
		out.FontSize = higher.FontSize
	}
	if higher.LineHeight.Set {
		out.LineHeight = higher.LineHeight
	}
	if higher.DisplayAlign != "" {
		out.DisplayAlign = higher.DisplayAlign
	}
	if higher.Overflow != "" {
		out.Overflow = higher.Overflow
	}
	if higher.WrapOption != "" {
		out.WrapOption = higher.WrapOption
	}
	if higher.Padding.Top.Set {
		out.Padding.Top = higher.Padding.Top
	}
	if higher.Padding.Right.Set {
		out.Padding.Right = higher.Padding.Right
	}
	if higher.Padding.Bottom.Set {
		out.Padding.Bottom = higher.Padding.Bottom
	}
	if higher.Padding.Left.Set {
		out.Padding.Left = higher.Padding.Left
	}
	return out
}

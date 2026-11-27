package imscfit

import (
	"encoding/xml"
	"strconv"
	"strings"
)

func parseStyling(dec *xml.Decoder, doc *Document) error {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			el := startElem{Local: t.Name.Local, Attrs: t.Copy().Attr}
			if el.Local == "style" {
				s := &Style{ID: el.attr("id"), Refs: el.attrStyleRefs(), Props: parseProps(el)}
				if s.ID != "" {
					doc.Styles[s.ID] = s
				}
			}
			if err := skip(dec); err != nil {
				return err
			}
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

func parseLayout(dec *xml.Decoder, doc *Document) error {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			el := startElem{Local: t.Name.Local, Attrs: t.Copy().Attr}
			if el.Local == "region" {
				r := &Region{ID: el.attr("id"), Refs: el.attrStyleRefs(), Props: parseProps(el)}
				if v := el.attr("origin"); v != "" {
					r.Origin, _ = parsePair(v)
				}
				if v := el.attr("extent"); v != "" {
					r.Extent, _ = parsePair(v)
				}
				if r.ID != "" {
					doc.Regions[r.ID] = r
				}
			}
			if err := skip(dec); err != nil {
				return err
			}
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

func parseProps(e startElem) Props {
	p := Props{
		DisplayAlign: e.attr("displayAlign"),
		Overflow:     e.attr("overflow"),
		WrapOption:   e.attr("wrapOption"),
	}
	if v := e.attr("fontSize"); v != "" {
		if l, err := ParseLength(v); err == nil {
			p.FontSize = l
		}
	}
	if v := e.attr("lineHeight"); v != "" {
		if l, err := ParseLength(v); err == nil {
			p.LineHeight = l
		}
	}
	if v := e.attr("padding"); v != "" {
		p.Padding = parsePadding(v)
	}
	return p
}

func parsePadding(v string) Padding {
	fields := strings.Fields(v)
	lengths := make([]Length, 0, 4)
	for _, f := range fields {
		if l, err := ParseLength(f); err == nil {
			lengths = append(lengths, l)
		}
	}
	out := Padding{}
	switch len(lengths) {
	case 1:
		out.Top, out.Right, out.Bottom, out.Left = lengths[0], lengths[0], lengths[0], lengths[0]
	case 2:
		out.Top, out.Bottom = lengths[0], lengths[0]
		out.Right, out.Left = lengths[1], lengths[1]
	case 3:
		out.Top, out.Right, out.Bottom = lengths[0], lengths[1], lengths[2]
		out.Left = lengths[1]
	case 4:
		out.Top, out.Right, out.Bottom, out.Left = lengths[0], lengths[1], lengths[2], lengths[3]
	}
	return out
}

func parseBody(dec *xml.Decoder, doc *Document, body startElem) error {
	frames := []contentFrame{{region: body.attr("region")}}
	return walkContent(dec, frames, func(p *Paragraph) {
		doc.Paragraphs = append(doc.Paragraphs, p)
	})
}

type contentFrame struct {
	region  string
	font    Length
	fontSet bool
}

// walkContent flattens div/p/span content. It owns every token up to and
// including the end element that matches its caller's start element.
func walkContent(dec *xml.Decoder, frames []contentFrame, emit func(*Paragraph)) error {
	var current *Paragraph
	fontStack := make([]Length, 0, 4)

	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.CharData:
			if current != nil {
				current.Segments = append(current.Segments, Segment{
					Text:     string(t),
					FontSize: effectiveFont(fontStack),
				})
			}
		case xml.StartElement:
			el := startElem{Local: t.Name.Local, Attrs: t.Copy().Attr}
			switch el.Local {
			case "div":
				f := contentFrame{region: el.attr("region")}
				if v, ok := lengthAttr(el, "fontSize"); ok {
					f.font, f.fontSet = v, true
				}
				if err := walkContent(dec, append(frames, f), emit); err != nil {
					return err
				}
			case "p":
				region := el.attr("region")
				if region == "" {
					region = nearestFrameRegion(frames)
				}
				current = newParagraph(el, region)
				fontStack = fontStack[:0]
				fontStack = append(fontStack, nearestFrameFont(frames))
				if v, ok := lengthAttr(el, "fontSize"); ok {
					fontStack = append(fontStack, v)
				}
				if err := walkParagraph(dec, current, &fontStack, emit); err != nil {
					return err
				}
				current = nil
			default:
				if err := skip(dec); err != nil {
					return err
				}
			}
		case xml.EndElement:
			return nil
		}
	}
}

// walkParagraph owns the body of a <p>, honoring nested span font sizes and
// <br/> forced breaks.
func walkParagraph(dec *xml.Decoder, p *Paragraph, fontStack *[]Length, emit func(*Paragraph)) error {
	depth := 1
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.CharData:
			p.Segments = append(p.Segments, Segment{Text: string(t), FontSize: effectiveFont(*fontStack)})
		case xml.StartElement:
			el := startElem{Local: t.Name.Local, Attrs: t.Copy().Attr}
			depth++
			switch el.Local {
			case "br":
				p.Segments = append(p.Segments, Segment{Break: true})
				if err := skip(dec); err != nil {
					return err
				}
				depth--
			case "span":
				*fontStack = append(*fontStack, elFontSize(el, *fontStack))
			default:
				if err := skip(dec); err != nil {
					return err
				}
				depth--
			}
		case xml.EndElement:
			depth--
			if depth == 0 {
				emit(p)
				return nil
			}
			*fontStack = popFont(*fontStack)
		}
	}
}

func popFont(stack []Length) []Length {
	if len(stack) <= 1 {
		return stack
	}
	return stack[:len(stack)-1]
}

func lengthAttr(el startElem, name string) (Length, bool) {
	if v := el.attr(name); v != "" {
		if l, err := ParseLength(v); err == nil {
			return l, true
		}
	}
	return Length{}, false
}

func nearestFrameRegion(frames []contentFrame) string {
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i].region != "" {
			return frames[i].region
		}
	}
	return ""
}

func nearestFrameFont(frames []contentFrame) Length {
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i].fontSet {
			return frames[i].font
		}
	}
	return Length{}
}

func newParagraph(el startElem, region string) *Paragraph {
	p := &Paragraph{
		ID:     el.attr("id"),
		Region: region,
		Begin:  el.attr("begin"),
		End:    el.attr("end"),
	}
	p.BeginSec, p.Timed = parseClock(p.Begin)
	if end, ok := parseClock(p.End); ok {
		p.EndSec = end
	}
	return p
}

func elFontSize(el startElem, stack []Length) Length {
	if v := el.attr("fontSize"); v != "" {
		if l, err := ParseLength(v); err == nil {
			return l
		}
	}
	return effectiveFont(stack)
}

func effectiveFont(stack []Length) Length {
	if len(stack) == 0 {
		return Length{}
	}
	return stack[len(stack)-1]
}

// parseClock accepts clock-time (HH:MM:SS(.mmm)), offset-time and frame
// expressions. Frames use the default frame rate of 30 when unavailable.
func parseClock(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	switch {
	case strings.HasSuffix(v, "ms"):
		if n, err := strconv.ParseFloat(strings.TrimSuffix(v, "ms"), 64); err == nil {
			return n / 1000, true
		}
	case strings.HasSuffix(v, "h"):
		if n, err := strconv.ParseFloat(strings.TrimSuffix(v, "h"), 64); err == nil {
			return n * 3600, true
		}
	case strings.HasSuffix(v, "m"):
		if n, err := strconv.ParseFloat(strings.TrimSuffix(v, "m"), 64); err == nil {
			return n * 60, true
		}
	case strings.HasSuffix(v, "s"):
		if n, err := strconv.ParseFloat(strings.TrimSuffix(v, "s"), 64); err == nil {
			return n, true
		}
	}
	parts := strings.Split(v, ":")
	total := 0.0
	for i, p := range parts {
		if i == 3 { // optional frame component, assumed 30fps
			if frames, err := strconv.ParseFloat(p, 64); err == nil {
				total += frames / 30
			}
			continue
		}
		n, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0, false
		}
		total = total*60 + n
	}
	return total, true
}

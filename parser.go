package imscfit

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	ttXMLNS  = "http://www.w3.org/ns/ttml"
	ttsXMLNS = "http://www.w3.org/ns/ttml#styling"
	ttpXMLNS = "http://www.w3.org/ns/ttml#parameter"
)

type startElem struct {
	Local string
	Space string
	Attrs []xml.Attr
}

func localName(n string) string {
	if i := strings.LastIndex(n, ":"); i >= 0 {
		return n[i+1:]
	}
	return n
}

func (e startElem) attr(local string) string {
	for _, a := range e.Attrs {
		if localName(a.Name.Local) == local {
			return a.Value
		}
	}
	return ""
}

func (e startElem) attrStyleRefs() []string {
	v := e.attr("style")
	if v == "" {
		return nil
	}
	return strings.Fields(v)
}

// Parse reads an IMSC/TTML stream.
func Parse(r io.Reader) (*Document, error) {
	dec := xml.NewDecoder(r)
	doc := newDocument()

	root, err := findRoot(dec)
	if err != nil {
		return nil, err
	}
	parseRootAttrs(doc, root)

	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			el := startElem{Local: t.Name.Local, Space: t.Name.Space, Attrs: t.Copy().Attr}
			depth++
			switch el.Local {
			case "head":
				if err := parseHead(dec, doc); err != nil {
					return nil, err
				}
				depth-- // head subtree consumed
			case "body":
				if err := parseBody(dec, doc, el); err != nil {
					return nil, err
				}
				depth--
			default:
				if err := skip(dec); err != nil {
					return nil, err
				}
				depth--
			}
		case xml.EndElement:
			depth--
		}
	}
	return doc, nil
}

func findRoot(dec *xml.Decoder) (startElem, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return startElem{}, fmt.Errorf("no TTML root element found: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			return startElem{Local: se.Name.Local, Space: se.Name.Space, Attrs: se.Copy().Attr}, nil
		}
	}
}

func parseRootAttrs(doc *Document, e startElem) {
	if v := e.attr("cellResolution"); v != "" {
		parts := strings.Fields(strings.ReplaceAll(v, "x", " "))
		if len(parts) == 2 {
			if x, err := strconv.Atoi(parts[0]); err == nil {
				doc.CellX = x
			}
			if y, err := strconv.Atoi(parts[1]); err == nil {
				doc.CellY = y
			}
		}
	}
	if v := e.attr("pixelAspectRatio"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			doc.PixelAspectRatio = f
		}
	}
	if v := e.attr("extent"); v != "" {
		if pair, err := parsePair(v); err == nil {
			doc.RootExtent = pair
		}
	}
}

func parsePair(v string) (LengthPair, error) {
	parts := strings.Fields(v)
	if len(parts) != 2 {
		return LengthPair{}, fmt.Errorf("expected two lengths in %q", v)
	}
	x, err := ParseLength(parts[0])
	if err != nil {
		return LengthPair{}, err
	}
	y, err := ParseLength(parts[1])
	if err != nil {
		return LengthPair{}, err
	}
	return LengthPair{X: x, Y: y}, nil
}

// skip consumes tokens until the matching end element of the current depth.
func skip(dec *xml.Decoder) error {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

func parseHead(dec *xml.Decoder, doc *Document) error {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			el := startElem{Local: t.Name.Local, Space: t.Name.Space, Attrs: t.Copy().Attr}
			switch el.Local {
			case "styling":
				if err := parseStyling(dec, doc); err != nil {
					return err
				}
			case "layout":
				if err := parseLayout(dec, doc); err != nil {
					return err
				}
			default:
				if err := skip(dec); err != nil {
					return err
				}
			}
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

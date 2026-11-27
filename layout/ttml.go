package layout

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

type xmlNode struct {
	Name     xml.Name
	Attrs    []xml.Attr
	Children []*xmlNode
	Parts    []xmlPart
	Text     string
}

type xmlPart struct {
	Text string
	Node *xmlNode
}

func (n *xmlNode) attr(local string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == local {
			return attr.Value
		}
	}
	return ""
}

type document struct {
	rootAttrs   map[string]string
	rootStyle   string
	styles      map[string]map[string]string
	regions     map[string]regionSpec
	regionOrder []string
	paragraphs  []paragraph
}

type regionSpec struct {
	ID     string
	Origin string
	Extent string
	Style  string
	Attrs  map[string]string
}

type paragraph struct {
	ID      string
	Region  string
	Style   string
	Begin   string
	End     string
	Attrs   map[string]string
	Content *xmlNode
}

func parseDocument(data []byte) (*document, error) {
	root, err := decodeXML(data)
	if err != nil {
		return nil, err
	}
	if root == nil || root.Name.Local != "tt" {
		return nil, errors.New("TTML root element must be <tt>")
	}

	doc := &document{
		rootAttrs: propertyAttrs(root),
		rootStyle: root.attr("style"),
		styles:    make(map[string]map[string]string),
		regions:   make(map[string]regionSpec),
	}
	walk(root, func(n *xmlNode) {
		switch n.Name.Local {
		case "style":
			id := elementID(n)
			if id != "" {
				props := propertyAttrs(n)
				if ref := n.attr("style"); ref != "" {
					props["style"] = ref
				}
				doc.styles[id] = props
			}
		case "region":
			id := elementID(n)
			if id == "" {
				id = n.attr("id")
			}
			if id != "" {
				if _, exists := doc.regions[id]; !exists {
					doc.regionOrder = append(doc.regionOrder, id)
				}
				doc.regions[id] = regionSpec{
					ID:     id,
					Origin: n.attr("origin"),
					Extent: n.attr("extent"),
					Style:  n.attr("style"),
					Attrs:  propertyAttrs(n),
				}
			}
		case "p":
			id := elementID(n)
			if id == "" {
				id = n.attr("id")
			}
			doc.paragraphs = append(doc.paragraphs, paragraph{
				ID:      id,
				Region:  n.attr("region"),
				Style:   n.attr("style"),
				Begin:   n.attr("begin"),
				End:     n.attr("end"),
				Attrs:   propertyAttrs(n),
				Content: n,
			})
		}
	})
	if len(doc.paragraphs) == 0 {
		return nil, errors.New("TTML contains no <p> paragraph")
	}
	return doc, nil
}

func decodeXML(data []byte) (*xmlNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var root *xmlNode
	var stack []*xmlNode
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode TTML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			node := &xmlNode{Name: value.Name, Attrs: value.Attr}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("TTML has multiple root elements")
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
				parent.Parts = append(parent.Parts, xmlPart{Node: node})
			}
			stack = append(stack, node)
		case xml.CharData:
			if len(stack) > 0 {
				node := stack[len(stack)-1]
				text := string(value)
				node.Text += text
				node.Parts = append(node.Parts, xmlPart{Text: text})
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("unexpected TTML closing element")
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) != 0 {
		return nil, errors.New("unclosed TTML element")
	}
	return root, nil
}

func walk(root *xmlNode, visit func(*xmlNode)) {
	if root == nil {
		return
	}
	visit(root)
	for _, child := range root.Children {
		walk(child, visit)
	}
}

func elementID(n *xmlNode) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == "id" {
			return attr.Value
		}
	}
	return ""
}

func propertyAttrs(n *xmlNode) map[string]string {
	props := make(map[string]string)
	for _, attr := range n.Attrs {
		switch attr.Name.Local {
		case "id", "style", "region", "begin", "end", "xml:lang", "lang":
			continue
		}
		props[attr.Name.Local] = attr.Value
	}
	return props
}

func (d *document) properties(styleRef string, inline map[string]string) map[string]string {
	props := make(map[string]string)
	d.mergeStyle(props, styleRef, make(map[string]bool))
	for key, value := range inline {
		props[key] = value
	}
	return props
}

func (d *document) mergeStyle(dst map[string]string, styleRef string, seen map[string]bool) {
	for _, ref := range strings.Fields(styleRef) {
		if seen[ref] {
			continue
		}
		seen[ref] = true
		style, ok := d.styles[ref]
		if !ok {
			continue
		}
		if parent := style["style"]; parent != "" {
			d.mergeStyle(dst, parent, seen)
		}
		for key, value := range style {
			if key != "style" {
				dst[key] = value
			}
		}
	}
}

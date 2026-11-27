package imsc

import (
	"encoding/xml"
	"io"
)

// Well-known IMSC-related XML namespaces.
const (
	NSXML   = "http://www.w3.org/XML/1998/namespace"
	NSTT    = "http://www.w3.org/ns/ttml"
	NSTTP   = "http://www.w3.org/ns/ttml#parameter"
	NSSTTS  = "http://www.w3.org/ns/ttml#styling"
	NSXMLNS = "http://www.w3.org/2000/xmlns/"
	NSSMPTE = "http://www.smpte-ra.org/schemas/2052-1/2010/smpte-tt"
	// NSExt is the converter's own namespace used to persist fallback links
	// across an export/re-import round trip.
	NSExt = "urn:imscconv:v1"
)

// Node is a namespace-aware generic XML element used both for parsing and
// for deterministic serialization.
type Node struct {
	Space string
	Local string
	// Attrs uses "{namespace}local" keys (Go encoding/xml convention).
	Attrs    []xml.Attr
	Children []*Node
	Text     string
	// Prefix bindings preserved for serialization.
	Prefixes map[string]string
}

// Name returns the Go-style "{space}local" element name.
func (n *Node) Name() string {
	if n.Space == "" {
		return n.Local
	}
	return "{" + n.Space + "}" + n.Local
}

// Attr returns the attribute value for "{space}local"; space may be "".
func (n *Node) Attr(space, local string) (string, bool) {
	want := local
	if space != "" {
		want = "{" + space + "}" + local
	}
	var bareValue string
	var bareOK bool
	for _, a := range n.Attrs {
		name := a.Name.Local
		if a.Name.Space != "" {
			name = "{" + a.Name.Space + "}" + a.Name.Local
		}
		if name == want {
			return a.Value, true
		}
		// Unprefixed attributes are not placed in the default namespace by
		// encoding/xml, so also accept the bare local name as a fallback.
		if space != "" && a.Name.Space == "" && a.Name.Local == local {
			bareValue = a.Value
			bareOK = true
		}
	}
	if bareOK {
		return bareValue, true
	}
	return "", false
}

// SetAttr sets an attribute, replacing an existing one with the same name.
func (n *Node) SetAttr(space, local, value string) {
	for i, a := range n.Attrs {
		if a.Name.Local == local && a.Name.Space == space {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, xml.Attr{
		Name:  xml.Name{Space: space, Local: local},
		Value: value,
	})
}

// Child returns the first child element with the given namespace and local name.
func (n *Node) Child(space, local string) *Node {
	for _, c := range n.Children {
		if c.Local == local && c.Space == space {
			return c
		}
	}
	return nil
}

// ChildrenAll returns every child element matching namespace and local name.
func (n *Node) ChildrenAll(space, local string) []*Node {
	var out []*Node
	for _, c := range n.Children {
		if c.Local == local && c.Space == space {
			out = append(out, c)
		}
	}
	return out
}

// DecodeXML reads a full document into the generic node tree.
func DecodeXML(r io.Reader) (*Node, error) {
	dec := xml.NewDecoder(r)
	root, err := decodeElement(dec, nil)
	if err != nil {
		return nil, err
	}
	return root, nil
}

func decodeElement(dec *xml.Decoder, parentPrefixes map[string]string) (*Node, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			node := &Node{
				Space:    t.Name.Space,
				Local:    t.Name.Local,
				Attrs:    append([]xml.Attr(nil), t.Attr...),
				Prefixes: map[string]string{},
			}
			for prefix, uri := range parentPrefixes {
				node.Prefixes[prefix] = uri
			}
			for _, a := range t.Attr {
				if a.Name.Space == NSXMLNS {
					node.Prefixes[a.Name.Local] = a.Value
				} else if a.Name.Local == "xmlns" && a.Name.Space == "" {
					node.Prefixes[""] = a.Value
				}
			}
			if err := decodeChildren(dec, node, t.Name); err != nil {
				return nil, err
			}
			return node, nil
		case xml.ProcInst, xml.Directive, xml.Comment:
			continue
		}
	}
}

func decodeChildren(dec *xml.Decoder, node *Node, start xml.Name) error {
	var text []byte
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child, err := decodeElementAt(dec, t, node.Prefixes)
			if err != nil {
				return err
			}
			node.Children = append(node.Children, child)
		case xml.CharData:
			text = append(text, t.Copy()...)
		case xml.EndElement:
			if t.Name == start {
				node.Text = string(text)
				return nil
			}
		}
	}
}

func decodeElementAt(dec *xml.Decoder, start xml.StartElement, parentPrefixes map[string]string) (*Node, error) {
	node := &Node{
		Space:    start.Name.Space,
		Local:    start.Name.Local,
		Attrs:    append([]xml.Attr(nil), start.Attr...),
		Prefixes: map[string]string{},
	}
	for prefix, uri := range parentPrefixes {
		node.Prefixes[prefix] = uri
	}
	for _, a := range start.Attr {
		if a.Name.Space == NSXMLNS {
			node.Prefixes[a.Name.Local] = a.Value
		} else if a.Name.Local == "xmlns" && a.Name.Space == "" {
			node.Prefixes[""] = a.Value
		}
	}
	return node, decodeChildren(dec, node, start.Name)
}

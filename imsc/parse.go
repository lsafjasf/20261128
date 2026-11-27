package imsc

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Parse decodes an IMSC image-profile TTML document and builds the resource
// hash table, region table, fallback chains and per-cue references.
func Parse(r io.Reader) (*Document, error) {
	root, err := DecodeXML(r)
	if err != nil {
		return nil, fmt.Errorf("imsc: parse failed: %w", err)
	}
	if root.Local != "tt" {
		return nil, fmt.Errorf("imsc: expected root <tt>, got <%s>", root.Local)
	}

	doc := NewDocument()
	if extent, ok := root.Attr(NSTTP, "extent"); ok {
		doc.Extent = extent
	}

	head := root.Child(NSTT, "head")
	if head != nil {
		if err := parseLayout(head, doc); err != nil {
			return nil, err
		}
		if err := parseEmbeddedImages(head, doc); err != nil {
			return nil, err
		}
	}

	body := root.Child(NSTT, "body")
	if body != nil {
		if err := parseCues(body, doc); err != nil {
			return nil, err
		}
	}

	doc.Conflicts = doc.Resources.Conflicts()
	return doc, nil
}

func parseLayout(head *Node, doc *Document) error {
	layout := head.Child(NSTT, "layout")
	if layout == nil {
		return nil
	}
	for _, node := range layout.ChildrenAll(NSTT, "region") {
		id, _ := node.Attr(NSXML, "id")
		if id == "" {
			continue
		}
		rg := &Region{
			ID:           id,
			Origin:       attrOr(node, NSSTTS, "origin"),
			Extent:       attrOr(node, NSSTTS, "extent"),
			DisplayAlign: attrOr(node, NSSTTS, "displayAlign"),
			TextAlign:    attrOr(node, NSSTTS, "textAlign"),
			OtherAttrs:   map[string]string{},
		}
		if fallback, ok := node.Attr(NSExt, "regionFallback"); ok {
			rg.FallbackID = fallback
		}
		var aliasIDs []string
		if aliases, ok := node.Attr(NSExt, "regionAliases"); ok {
			aliasIDs = strings.Fields(aliases)
		}
		for _, a := range node.Attrs {
			name := attrName(a)
			if isCoreRegionAttr(name) || name == "{"+NSExt+"}regionAliases" {
				continue
			}
			if a.Name.Space == NSXMLNS || a.Name.Local == "xmlns" {
				continue
			}
			rg.OtherAttrs[name] = a.Value
			if a.Name.Space != "" && !isBuiltinNamespace(a.Name.Space) {
				doc.registerNamespace(a.Name.Space)
			}
		}
		canonical, _ := doc.Regions.Register(rg)
		for _, alias := range aliasIDs {
			if alias == canonical.ID || containsString(canonical.Aliases, alias) {
				continue
			}
			canonical.Aliases = append(canonical.Aliases, alias)
			doc.Regions.alias(alias, canonical)
		}
	}
	return nil
}

func parseEmbeddedImages(head *Node, doc *Document) error {
	// smpte:image elements live directly under head in the SMPTE namespace.
	for _, node := range head.ChildrenAll(NSSMPTE, "image") {
		id, ok := node.Attr(NSXML, "id")
		if !ok {
			continue
		}
		mediaType, _ := node.Attr(NSSMPTE, "imageType")
		encoding, _ := node.Attr(NSSMPTE, "encoding")
		raw := strings.TrimSpace(node.Text)
		data, err := decodeEmbedded(raw, encoding)
		if err != nil {
			return fmt.Errorf("imsc: smpte:image %q: %w", id, err)
		}
		uri := "#" + id
		doc.Resources.RegisterContent(uri, KindEmbedded, mediaType, data, id)
		if doc.SourceKind == KindExternal {
			doc.SourceKind = KindEmbedded
		}
	}
	return nil
}

func decodeEmbedded(raw, encoding string) ([]byte, error) {
	if encoding != "" && encoding != "Base64" && encoding != "base64" {
		return nil, fmt.Errorf("unsupported smpte:encoding %q", encoding)
	}
	return base64.StdEncoding.DecodeString(stripWhitespace(raw))
}

func parseCues(body *Node, doc *Document) error {
	for _, div := range body.ChildrenAll(NSTT, "div") {
		for _, p := range div.ChildrenAll(NSTT, "p") {
			cue := &Cue{
				Begin:    attrOr(p, NSTT, "begin"),
				End:      attrOr(p, NSTT, "end"),
				RegionID: attrOr(p, NSTT, "region"),
			}
			cue.ID, _ = p.Attr(NSXML, "id")
			if fb, ok := p.Attr(NSExt, "imageFallback"); ok {
				cue.FallbackURI = fb
			}

			img := p.Child(NSTT, "image")
			switch {
			case img != nil:
				uri, _ := img.Attr("", "src")
				if fb, ok := img.Attr(NSExt, "fallback"); ok && cue.FallbackURI == "" {
					cue.FallbackURI = fb
				}
				if err := bindCueImage(cue, uri, doc); err != nil {
					return err
				}
			default:
				if bg, ok := p.Attr(NSSMPTE, "backgroundImage"); ok {
					if err := bindCueImage(cue, bg, doc); err != nil {
						return err
					}
				}
			}

			if img == nil {
				cue.Text = strings.TrimSpace(p.Text)
			}
			if _, err := doc.Regions.AddRef(cue.RegionID); err != nil {
				return err
			}
			doc.Cues = append(doc.Cues, cue)
		}
	}
	return nil
}

func bindCueImage(cue *Cue, uri string, doc *Document) error {
	cue.ImageURI = uri
	switch {
	case strings.HasPrefix(uri, "data:"):
		mediaType, data, err := ParseDataURI(uri)
		if err != nil {
			return err
		}
		res, _ := doc.Resources.RegisterContent(uri, KindDataURI, mediaType, data, "")
		cue.ResourceKey = res.Key
		if doc.SourceKind == KindExternal {
			doc.SourceKind = KindDataURI
		}
	case strings.HasPrefix(uri, "#"):
		res, ok := doc.Resources.Lookup(uri)
		if !ok {
			return fmt.Errorf("imsc: cue %q references missing embedded image %q", cue.ID, uri)
		}
		cue.ResourceKey = res.Key
	default:
		res := doc.Resources.RegisterExternal(uri)
		cue.ResourceKey = res.Key
	}
	if cue.FallbackURI != "" {
		doc.Resources.SetFallback(uri, cue.FallbackURI)
	}
	return nil
}

func attrOr(n *Node, space, local string) string {
	v, _ := n.Attr(space, local)
	return v
}

func attrName(a xml.Attr) string {
	if a.Name.Space == "" {
		return a.Name.Local
	}
	return "{" + a.Name.Space + "}" + a.Name.Local
}

func isCoreRegionAttr(name string) bool {
	switch name {
	case "{" + NSXML + "}id",
		"{" + NSSTTS + "}origin",
		"{" + NSSTTS + "}extent",
		"{" + NSSTTS + "}displayAlign",
		"{" + NSSTTS + "}textAlign",
		"{" + NSExt + "}regionFallback":
		return true
	}
	return false
}

func stripWhitespace(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, s)
}

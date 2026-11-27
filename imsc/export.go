package imsc

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

// prefixFor maps a namespace URI to the prefix used on export.
var prefixFor = map[string]string{
	NSTT:    "tt",
	NSTTP:   "ttp",
	NSSTTS:  "tts",
	NSSMPTE: "smpte",
	NSExt:   "conv",
}

// uriForPrefix is the reverse binding written on the root element.
var uriForPrefix = map[string]string{
	"tt":    NSTT,
	"ttp":   NSTTP,
	"tts":   NSSTTS,
	"smpte": NSSMPTE,
	"conv":  NSExt,
	"xml":   NSXML,
}

// ExportOptions controls how Document.Export serializes resources.
type ExportOptions struct {
	// Embedded emits every payload once as an smpte:image element and has
	// cues reference it via fragment URI. This is the default and the most
	// space-efficient form.
	Embedded bool
	// DataURI inlines a data: URI on each cue instead of using smpte:image.
	// Equal payloads still resolve to the same Resource key; on re-import
	// they collapse to one hash-table entry again.
	DataURI bool
}

// DefaultExportOptions embeds each distinct payload exactly once.
func DefaultExportOptions() ExportOptions {
	return ExportOptions{Embedded: true}
}

// Export serializes the document to IMSC TTML, preserving every reference
// relationship instead of copying bytes per cue.
func (d *Document) Export(w io.Writer, opts ExportOptions) error {
	if opts.Embedded == opts.DataURI {
		opts = DefaultExportOptions()
	}
	buf := &bytes.Buffer{}
	buf.WriteString(xml.Header)

	enc := xml.NewEncoder(buf)
	enc.Indent("", "  ")

	root := xml.StartElement{
		Name: qname(NSTT, "tt"),
		Attr: append(nsDecls(d),
			xml.Attr{Name: xml.Name{Local: "ttp:extent"}, Value: orDefault(d.Extent, "1920px 1080px")},
		),
	}
	if err := enc.EncodeToken(root); err != nil {
		return err
	}

	if err := d.writeHead(enc, opts); err != nil {
		return err
	}
	if err := d.writeBody(enc, opts); err != nil {
		return err
	}

	if err := enc.EncodeToken(root.End()); err != nil {
		return err
	}
	if err := enc.Flush(); err != nil {
		return err
	}
	buf.WriteByte('\n')
	_, err := w.Write(buf.Bytes())
	return err
}

func nsDecls(d *Document) []xml.Attr {
	prefixes := make([]string, 0, len(uriForPrefix))
	for prefix := range uriForPrefix {
		if prefix == "xml" {
			continue
		}
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	attrs := make([]xml.Attr, 0, len(prefixes)+len(d.ExtraNamespaces))
	for _, prefix := range prefixes {
		attrs = append(attrs, xml.Attr{
			Name:  xml.Name{Local: "xmlns:" + prefix},
			Value: uriForPrefix[prefix],
		})
	}
	extraURIs := make([]string, 0, len(d.ExtraNamespaces))
	for uri := range d.ExtraNamespaces {
		extraURIs = append(extraURIs, uri)
	}
	sort.Strings(extraURIs)
	for _, uri := range extraURIs {
		attrs = append(attrs, xml.Attr{
			Name:  xml.Name{Local: "xmlns:" + d.ExtraNamespaces[uri]},
			Value: uri,
		})
	}
	return attrs
}

func qname(space, local string) xml.Name {
	prefix, ok := prefixFor[space]
	if !ok {
		return xml.Name{Space: space, Local: local}
	}
	return xml.Name{Local: prefix + ":" + local}
}

func attr(space, local, value string) xml.Attr {
	prefix, ok := prefixFor[space]
	name := local
	if ok {
		name = prefix + ":" + local
	}
	return xml.Attr{Name: xml.Name{Local: name}, Value: value}
}

func xmlID(value string) xml.Attr {
	return xml.Attr{Name: xml.Name{Local: "xml:id"}, Value: value}
}

func encodeAll(enc *xml.Encoder, tokens ...xml.Token) error {
	for _, token := range tokens {
		if err := enc.EncodeToken(token); err != nil {
			return err
		}
	}
	return nil
}

func (d *Document) writeHead(enc *xml.Encoder, opts ExportOptions) error {
	head := xml.StartElement{Name: qname(NSTT, "head")}
	if err := enc.EncodeToken(head); err != nil {
		return err
	}

	layout := xml.StartElement{Name: qname(NSTT, "layout")}
	if err := enc.EncodeToken(layout); err != nil {
		return err
	}
	for _, rg := range d.Regions.All() {
		if rg.ID == DefaultRegionID && rg.Origin == "" && rg.Extent == "" &&
			rg.DisplayAlign == "" && rg.TextAlign == "" && len(rg.OtherAttrs) == 0 {
			continue
		}
		start := xml.StartElement{Name: qname(NSTT, "region")}
		start.Attr = append(start.Attr, xmlID(rg.ID))
		addIf := func(space, local, value string) {
			if value != "" {
				start.Attr = append(start.Attr, attr(space, local, value))
			}
		}
		addIf(NSSTTS, "origin", rg.Origin)
		addIf(NSSTTS, "extent", rg.Extent)
		addIf(NSSTTS, "displayAlign", rg.DisplayAlign)
		addIf(NSSTTS, "textAlign", rg.TextAlign)
		addIf(NSExt, "regionFallback", rg.FallbackID)
		if len(rg.Aliases) > 1 {
			addIf(NSExt, "regionAliases", strings.Join(rg.Aliases, " "))
		}
		keys := make([]string, 0, len(rg.OtherAttrs))
		for key := range rg.OtherAttrs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: d.otherAttrName(key)}, Value: rg.OtherAttrs[key]})
		}
		if err := encodeAll(enc, start, start.End()); err != nil {
			return err
		}
	}
	if err := enc.EncodeToken(layout.End()); err != nil {
		return err
	}

	if opts.Embedded {
		for _, res := range d.Resources.Resources() {
			if len(res.Data) == 0 {
				continue
			}
			id := embeddedID(res)
			start := xml.StartElement{Name: qname(NSSMPTE, "image")}
			start.Attr = append(start.Attr, xmlID(id))
			if res.MediaType != "" {
				start.Attr = append(start.Attr, attr(NSSMPTE, "imageType", res.MediaType))
			}
			start.Attr = append(start.Attr, attr(NSSMPTE, "encoding", "Base64"))
			encoded := base64.StdEncoding.EncodeToString(res.Data)
			if err := encodeAll(enc, start, xml.CharData(encoded), start.End()); err != nil {
				return err
			}
		}
	}

	return enc.EncodeToken(head.End())
}

func (d *Document) writeBody(enc *xml.Encoder, opts ExportOptions) error {
	body := xml.StartElement{Name: qname(NSTT, "body")}
	if err := enc.EncodeToken(body); err != nil {
		return err
	}
	div := xml.StartElement{Name: qname(NSTT, "div")}
	div.Attr = append(div.Attr, xmlID("images"))
	if err := enc.EncodeToken(div); err != nil {
		return err
	}

	for i, cue := range d.Cues {
		p := xml.StartElement{Name: qname(NSTT, "p")}
		if cue.ID != "" {
			p.Attr = append(p.Attr, xmlID(cue.ID))
		} else {
			p.Attr = append(p.Attr, xmlID(fmt.Sprintf("cue%d", i+1)))
		}
		p.Attr = append(p.Attr, attr(NSTT, "begin", cue.Begin))
		p.Attr = append(p.Attr, attr(NSTT, "end", cue.End))
		regionID := cue.RegionID
		if regionID == "" {
			regionID = DefaultRegionID
		}
		p.Attr = append(p.Attr, attr(NSTT, "region", regionID))

		res, ok := d.Resources.LookupKey(cue.ResourceKey)
		if !ok {
			return fmt.Errorf("imsc: cue %q references missing resource key %q", cue.ID, cue.ResourceKey)
		}

		if opts.DataURI && len(res.Data) > 0 {
			img := xml.StartElement{Name: qname(NSTT, "image")}
			img.Attr = append(img.Attr, xml.Attr{
				Name:  xml.Name{Local: "src"},
				Value: EncodeDataURI(res.MediaType, res.Data),
			})
			if cue.FallbackURI != "" {
				img.Attr = append(img.Attr, attr(NSExt, "fallback", cue.FallbackURI))
			}
			if err := encodeAll(enc, p, img, img.End(), p.End()); err != nil {
				return err
			}
			continue
		}

		if len(res.Data) > 0 {
			p.Attr = append(p.Attr, attr(NSSMPTE, "backgroundImage", "#"+embeddedID(res)))
		} else {
			p.Attr = append(p.Attr, attr(NSSMPTE, "backgroundImage", res.URI()))
		}
		if cue.FallbackURI != "" {
			p.Attr = append(p.Attr, attr(NSExt, "imageFallback", cue.FallbackURI))
		}
		var tokens []xml.Token
		tokens = append(tokens, p)
		if cue.Text != "" {
			tokens = append(tokens, xml.CharData(cue.Text))
		}
		tokens = append(tokens, p.End())
		if err := encodeAll(enc, tokens...); err != nil {
			return err
		}
	}

	if err := enc.EncodeToken(div.End()); err != nil {
		return err
	}
	return enc.EncodeToken(body.End())
}

// embeddedID returns the stable smpte:image xml:id for an embedded resource.
// The hash-derived name guarantees two cues pointing at identical bytes share
// one image element after any export/re-import cycle.
func embeddedID(res *Resource) string {
	if res.EmbeddedID != "" {
		return res.EmbeddedID
	}
	return "img-" + res.Hash[:12]
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// otherAttrName converts a "{namespace}local" key back into a prefixed
// "prefix:local" attribute name using built-in and document bindings.
func (d *Document) otherAttrName(key string) string {
	if len(key) == 0 || key[0] != '{' {
		return key
	}
	close := strings.IndexByte(key, '}')
	space := key[1:close]
	local := key[close+1:]
	if prefix, ok := prefixFor[space]; ok {
		return prefix + ":" + local
	}
	if prefix, ok := d.ExtraNamespaces[space]; ok {
		return prefix + ":" + local
	}
	return local
}

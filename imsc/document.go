package imsc

import "fmt"

// Cue is one timed image entry (an IMSC <p> carrying an <image> or an
// smpte:backgroundImage attribute). Cues never own image bytes; they point
// at a Resource by canonical key and at a Region by region ID.
type Cue struct {
	ID          string
	Begin       string
	End         string
	RegionID    string
	ImageURI    string // source URI as authored (data:, file URI or fragment)
	ResourceKey string
	// FallbackURI is the converter-namespaced fallback, if any.
	FallbackURI string
	Text        string
}

// Document is the normalized in-memory representation of an IMSC image
// subtitle document.
type Document struct {
	Resources *Registry
	Regions   *RegionTable
	Cues      []*Cue

	// Extent is the ttp:extent of the root tt element.
	Extent string
	// SourceKind records whether images arrived as data URIs or smpte:image.
	SourceKind ResourceKind
	// Conflicts is a convenience snapshot of Registry.Conflicts after parse.
	Conflicts []URIConflict
	// ExtraNamespaces maps namespace URIs discovered on region attributes
	// (outside the built-in IMSC set) to stable generated prefixes, so they
	// can be re-declared on export and survive a round trip.
	ExtraNamespaces map[string]string
}

// NewDocument returns an empty document with initialized tables.
func NewDocument() *Document {
	return &Document{
		Resources:       NewRegistry(),
		Regions:         NewRegionTable(),
		ExtraNamespaces: map[string]string{},
	}
}

// CueCount returns the number of cues.
func (d *Document) CueCount() int { return len(d.Cues) }

// isBuiltinNamespace reports whether uri is one of the namespaces the
// exporter already declares with a fixed prefix.
func isBuiltinNamespace(uri string) bool {
	switch uri {
	case NSTT, NSTTP, NSSTTS, NSSMPTE, NSExt, NSXML:
		return true
	}
	return false
}

// registerNamespace records an extra namespace with a stable generated prefix.
func (d *Document) registerNamespace(uri string) {
	if _, ok := d.ExtraNamespaces[uri]; ok {
		return
	}
	d.ExtraNamespaces[uri] = fmt.Sprintf("ns%d", len(d.ExtraNamespaces)+1)
}

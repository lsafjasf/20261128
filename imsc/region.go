package imsc

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// Region mirrors an IMSC <region> element. Cues reference regions by ID;
// identical region definitions are shared instead of being copied per cue.
type Region struct {
	// ID is the stable xml:id used inside the document.
	ID string
	// Origin is the tts:origin expression ("left top"). Empty means inherit.
	Origin string
	// Extent is the tts:extent expression ("width height"). Empty means inherit.
	Extent string
	// DisplayAlign is tts:displayAlign (before|center|after).
	DisplayAlign string
	// TextAlign is tts:textAlign (start|center|end).
	TextAlign string
	// OtherAttrs carries additional namespace-qualified attributes
	// ("{namespace}local" -> value) so round-tripping keeps them intact.
	OtherAttrs map[string]string
	// FallbackID names the region whose properties apply when this region is
	// missing during resolution. Empty means the default region terminates
	// the chain.
	FallbackID string
	// Aliases lists every xml:id that was observed with this exact structure,
	// first-seen first. The ID field holds the canonical (first) alias.
	Aliases []string
	// RefCount is the number of cues resolved against this region.
	RefCount int
}

// RegionTable de-duplicates region definitions by structural hash and keeps
// the region fallback graph.
type RegionTable struct {
	byID   map[string]*Region
	byHash map[string]string // structural hash -> canonical region ID
	order  []string          // canonical IDs in first-seen order
}

// NewRegionTable creates an empty table and installs the IMSC default region
// (no explicit placement properties), which terminates every fallback chain.
func NewRegionTable() *RegionTable {
	t := &RegionTable{
		byID:   make(map[string]*Region),
		byHash: make(map[string]string),
	}
	t.Register(&Region{ID: DefaultRegionID, Aliases: []string{DefaultRegionID}})
	return t
}

// DefaultRegionID is the xml:id of the implicit IMSC default region.
const DefaultRegionID = "defaultRegion"

// fingerprint is the structural hash of every placement-relevant property.
// Two regions with the same fingerprint are safe to merge into one reference.
func (rg *Region) fingerprint() string {
	h := sha256.New()
	write := func(label, value string) {
		h.Write([]byte(label))
		h.Write([]byte{0})
		h.Write([]byte(value))
		h.Write([]byte{0})
	}
	write("origin", rg.Origin)
	write("extent", rg.Extent)
	write("displayAlign", rg.DisplayAlign)
	write("textAlign", rg.TextAlign)
	keys := make([]string, 0, len(rg.OtherAttrs))
	for key := range rg.OtherAttrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		write("attr:"+key, rg.OtherAttrs[key])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Register inserts a region or merges it into an existing structurally equal
// region. The returned region is the canonical instance; merged reports
// whether an existing definition was reused. A later ID observed with the
// same structure becomes an alias rather than creating a duplicate region.
func (t *RegionTable) Register(rg *Region) (canonical *Region, merged bool) {
	if rg.ID == "" {
		rg.ID = DefaultRegionID
	}
	if rg.Aliases == nil {
		rg.Aliases = []string{rg.ID}
	}
	if rg.ID == DefaultRegionID {
		t.byID[DefaultRegionID] = rg
		return rg, false
	}
	fp := rg.fingerprint()
	if existingID, ok := t.byHash[fp]; ok {
		existing := t.byID[existingID]
		if !containsString(existing.Aliases, rg.ID) {
			existing.Aliases = append(existing.Aliases, rg.ID)
		}
		t.byID[rg.ID] = existing
		if rg.FallbackID != "" && existing.FallbackID == "" {
			existing.FallbackID = rg.FallbackID
		}
		return existing, true
	}
	if _, exists := t.byID[rg.ID]; !exists {
		t.order = append(t.order, rg.ID)
	}
	t.byID[rg.ID] = rg
	t.byHash[fp] = rg.ID
	return rg, false
}

// Get returns the region registered with id, honoring alias merges.
func (t *RegionTable) Get(id string) (*Region, bool) {
	if id == "" {
		id = DefaultRegionID
	}
	rg, ok := t.byID[id]
	return rg, ok
}

// alias points an additional id at an already-registered canonical region.
func (t *RegionTable) alias(id string, canonical *Region) {
	if _, exists := t.byID[id]; !exists {
		t.byID[id] = canonical
	}
}

// All returns canonical regions in first-seen order.
func (t *RegionTable) All() []*Region {
	out := make([]*Region, 0, len(t.order))
	for _, id := range t.order {
		if rg, ok := t.byID[id]; ok && rg.ID == id {
			out = append(out, rg)
		}
	}
	return out
}

// Len reports the number of canonical region definitions.
func (t *RegionTable) Len() int { return len(t.All()) }

// SetFallback declares that regionID falls back to fallbackID when it cannot
// be resolved.
func (t *RegionTable) SetFallback(regionID, fallbackID string) {
	if rg, ok := t.byID[regionID]; ok {
		rg.FallbackID = fallbackID
	}
}

// Chain returns the region fallback chain starting at id (empty id starts at
// the default region). Each node appears once; cycles stop the walk.
func (t *RegionTable) Chain(id string) []string {
	if id == "" {
		id = DefaultRegionID
	}
	seen := map[string]bool{}
	var chain []string
	current := id
	for {
		if seen[current] {
			return chain
		}
		seen[current] = true
		chain = append(chain, current)
		rg, ok := t.byID[current]
		if !ok || rg.FallbackID == "" {
			return chain
		}
		current = rg.FallbackID
	}
}

// Resolve follows fallback links until a registered region is found. The
// default region always terminates the chain. A cycle produces an error
// alongside the last reachable region.
func (t *RegionTable) Resolve(id string) (*Region, error) {
	if id == "" {
		id = DefaultRegionID
	}
	seen := map[string]bool{}
	current := id
	for {
		if seen[current] {
			if rg, ok := t.byID[current]; ok {
				return rg, &CycleError{Kind: "region", ID: current}
			}
			return t.byID[DefaultRegionID], &CycleError{Kind: "region", ID: current}
		}
		seen[current] = true
		rg, ok := t.byID[current]
		if !ok {
			// Unknown region: the default region always terminates the chain.
			if current == DefaultRegionID {
				return t.byID[DefaultRegionID], nil
			}
			current = DefaultRegionID
			continue
		}
		if rg.FallbackID == "" {
			return rg, nil
		}
		current = rg.FallbackID
	}
}

// AddRef resolves id (including fallback) and increments its reference count.
func (t *RegionTable) AddRef(id string) (*Region, error) {
	rg, err := t.Resolve(id)
	if rg != nil {
		rg.RefCount++
	}
	return rg, err
}

// CycleError is returned when a fallback chain loops back on itself.
type CycleError struct {
	Kind string
	ID   string
}

func (e *CycleError) Error() string {
	return "imsc: " + e.Kind + " fallback chain cycle detected at " + e.ID
}

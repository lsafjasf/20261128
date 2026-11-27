package imsc

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"net/url"
	"path"
	"sort"
	"strings"
)

// ResourceKind describes how an image resource is carried inside the document.
type ResourceKind int

const (
	// KindExternal references an external file URI whose bytes are not embedded.
	KindExternal ResourceKind = iota
	// KindDataURI is carried inline as a data: URI.
	KindDataURI
	// KindEmbedded is carried inline as a base64 smpte:image element.
	KindEmbedded
)

func (k ResourceKind) String() string {
	switch k {
	case KindDataURI:
		return "data-uri"
	case KindEmbedded:
		return "embedded"
	default:
		return "external"
	}
}

// Resource is a single image payload addressed by its content hash.
// Multiple cues may reference the same Resource without copying its bytes.
type Resource struct {
	// Key is the canonical lookup key: the sha256 hex of the payload, or
	// "ext:<uri>" for an external resource whose content is unavailable.
	Key string
	// Hash is the sha256 hex of the decoded payload; empty for external
	// resources with unknown content.
	Hash string
	// URIs lists every URI that was observed pointing at this exact content,
	// in first-seen order and de-duplicated. The first entry is canonical.
	URIs      []string
	Kind      ResourceKind
	MediaType string
	// Data holds the decoded image bytes; nil for unresolved external resources.
	Data []byte
	// EmbeddedID is the xml:id of the smpte:image element when Kind == KindEmbedded.
	EmbeddedID string
}

// URI returns the canonical (first observed) URI for the resource.
func (r *Resource) URI() string {
	if len(r.URIs) == 0 {
		return ""
	}
	return r.URIs[0]
}

// URIConflict records that one URI was observed with two different payloads.
// Such resources must never be merged.
type URIConflict struct {
	URI             string
	FirstKey        string
	ConflictingKey  string
	FirstMediaType  string
	SecondMediaType string
}

// externalPrefix namespaces keys for external resources whose bytes are unknown.
const externalPrefix = "ext:"

// Registry is the content-addressed resource hash table.
type Registry struct {
	byHash    map[string]*Resource
	byURI     map[string]string // URI -> canonical key; first writer wins
	conflicts []URIConflict
	fallbacks map[string]string // explicit fallback: preferred URI -> fallback URI
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		byHash:    make(map[string]*Resource),
		byURI:     make(map[string]string),
		fallbacks: make(map[string]string),
	}
}

// RegisterContent records an image payload reached via uri. The payload is
// content-addressed by sha256. When the same URI was previously seen with a
// different payload the merge is blocked: the URI keeps pointing at the first
// resource and a URIConflict is recorded. The conflicting payload is still
// stored under its own hash so every cue keeps its real bytes.
func (r *Registry) RegisterContent(uri string, kind ResourceKind, mediaType string, data []byte, embeddedID string) (*Resource, *URIConflict) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	res, ok := r.byHash[hash]
	if !ok {
		res = &Resource{
			Key:        hash,
			Hash:       hash,
			Kind:       kind,
			MediaType:  mediaType,
			Data:       append([]byte(nil), data...),
			EmbeddedID: embeddedID,
		}
		r.byHash[hash] = res
	} else if mediaType != "" {
		if res.MediaType == "" {
			res.MediaType = mediaType
		}
		if embeddedID != "" && res.EmbeddedID == "" {
			res.EmbeddedID = embeddedID
		}
	}

	conflict := r.bindURI(uri, res)
	return res, conflict
}

// RegisterExternal records an external URI whose bytes are not embedded.
// External resources without known content cannot be hash-merged with
// anything; each distinct URI gets its own key.
func (r *Registry) RegisterExternal(uri string) *Resource {
	key := externalPrefix + uri
	if res, ok := r.byHash[key]; ok {
		r.bindURI(uri, res)
		return res
	}
	res := &Resource{
		Key:       key,
		Kind:      KindExternal,
		MediaType: mediaTypeFromURI(uri),
	}
	r.byHash[key] = res
	r.bindURI(uri, res)
	return res
}

// bindURI associates uri with res. First writer wins; a later, different
// content produces a conflict instead of overwriting the binding.
func (r *Registry) bindURI(uri string, res *Resource) *URIConflict {
	if uri != "" {
		if existing, ok := r.byURI[uri]; ok {
			if existing != res.Key {
				first := r.byHash[existing]
				conflict := &URIConflict{
					URI:             uri,
					FirstKey:        existing,
					ConflictingKey:  res.Key,
					FirstMediaType:  first.MediaType,
					SecondMediaType: res.MediaType,
				}
				r.conflicts = append(r.conflicts, *conflict)
				return conflict
			}
		} else {
			r.byURI[uri] = res.Key
		}
		if !containsString(res.URIs, uri) {
			res.URIs = append(res.URIs, uri)
		}
	}
	return nil
}

// Lookup returns the resource registered for uri, honoring the first-writer
// binding even when a conflict was observed.
func (r *Registry) Lookup(uri string) (*Resource, bool) {
	key, ok := r.byURI[uri]
	if !ok {
		return nil, false
	}
	res, ok := r.byHash[key]
	return res, ok
}

// LookupKey returns the resource with the given canonical key.
func (r *Registry) LookupKey(key string) (*Resource, bool) {
	res, ok := r.byHash[key]
	return res, ok
}

// Conflicts returns all blocked same-URI/different-content merges.
func (r *Registry) Conflicts() []URIConflict {
	out := make([]URIConflict, len(r.conflicts))
	copy(out, r.conflicts)
	return out
}

// HasConflict reports whether uri was seen with differing content.
func (r *Registry) HasConflict(uri string) bool {
	for _, c := range r.conflicts {
		if c.URI == uri {
			return true
		}
	}
	return false
}

// Resources returns all resources sorted by canonical key for deterministic output.
func (r *Registry) Resources() []*Resource {
	keys := make([]string, 0, len(r.byHash))
	for key := range r.byHash {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*Resource, 0, len(keys))
	for _, key := range keys {
		out = append(out, r.byHash[key])
	}
	return out
}

// Len reports the number of distinct payloads in the hash table.
func (r *Registry) Len() int { return len(r.byHash) }

// SetFallback declares that when preferredURI is unavailable the converter
// should try fallbackURI. The chain is resolved with Resolve, which also
// detects cycles.
func (r *Registry) SetFallback(preferredURI, fallbackURI string) {
	if preferredURI == "" || fallbackURI == "" || preferredURI == fallbackURI {
		return
	}
	r.fallbacks[preferredURI] = fallbackURI
}

// Fallback returns the configured fallback URI for preferredURI, if any.
func (r *Registry) Fallback(preferredURI string) (string, bool) {
	uri, ok := r.fallbacks[preferredURI]
	return uri, ok
}

// Resolve walks the resource fallback chain starting at uri and returns the
// first URI that is registered with known content (or any registered entry
// for external URIs). The original uri is returned unchanged when no link is
// configured. A cycle terminates at the repeated node and an error is
// returned alongside the best reachable URI.
func (r *Registry) Resolve(uri string) (string, error) {
	seen := map[string]bool{}
	current := uri
	for {
		if seen[current] {
			return current, fmt.Errorf("imsc: fallback chain cycle detected at %q", current)
		}
		seen[current] = true
		if res, ok := r.Lookup(current); ok && (res.Kind == KindExternal || len(res.Data) > 0) {
			return current, nil
		}
		next, ok := r.fallbacks[current]
		if !ok {
			return current, nil
		}
		current = next
	}
}

// Chain returns the ordered fallback chain starting at uri, including uri
// itself, stopping before any cycle repeat.
func (r *Registry) Chain(uri string) []string {
	seen := map[string]bool{}
	var chain []string
	current := uri
	for {
		if seen[current] {
			return chain
		}
		seen[current] = true
		chain = append(chain, current)
		next, ok := r.fallbacks[current]
		if !ok {
			return chain
		}
		current = next
	}
}

// ParseDataURI decodes an RFC 2397 data URI into media type and payload.
func ParseDataURI(raw string) (mediaType string, data []byte, err error) {
	if !strings.HasPrefix(raw, "data:") {
		return "", nil, fmt.Errorf("imsc: not a data URI: %q", truncate(raw))
	}
	comma := strings.IndexByte(raw, ',')
	if comma < 0 {
		return "", nil, fmt.Errorf("imsc: malformed data URI: missing comma")
	}
	meta := raw[len("data:"):comma]
	payload := raw[comma+1:]
	base64Encoded := false
	if meta != "" {
		parts := strings.Split(meta, ";")
		for i, part := range parts {
			if part == "base64" {
				base64Encoded = true
				continue
			}
			if i == 0 && !strings.Contains(part, "=") {
				mediaType = part
				continue
			}
		}
	}
	if mediaType == "" {
		mediaType = "text/plain;charset=US-ASCII"
	}
	if base64Encoded {
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", nil, fmt.Errorf("imsc: data URI base64 decode failed: %w", err)
		}
	} else {
		unquoted, derr := url.PathUnescape(payload)
		if derr != nil {
			return "", nil, fmt.Errorf("imsc: data URI unescape failed: %w", derr)
		}
		data = []byte(unquoted)
	}
	return mediaType, data, nil
}

// EncodeDataURI renders payload as a base64 data URI.
func EncodeDataURI(mediaType string, data []byte) string {
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func mediaTypeFromURI(uri string) string {
	ext := path.Ext(uri)
	if ext == "" {
		return ""
	}
	if mt := mime.TypeByExtension(ext); mt != "" {
		return mt
	}
	return ""
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func truncate(s string) string {
	const max = 48
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

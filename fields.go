package wavinfo

// Standard INFO list field identifiers, in the authoritative order defined by
// the RIFF specification (Microsoft Multimedia Standards Update, 1991) and
// reproduced by the New Data Types chapter (New Chunk Types, "LIST 'INFO'").
//
// This slice is the single source of truth for serialization order: known
// fields are always emitted in this order; fields not present here keep their
// original relative order and are emitted after the known ones.
var canonicalOrder = []string{
	"IARL", // Archival Location
	"IART", // Artist
	"ICMS", // Commissioned
	"ICMP", // Compression Method (note: distinct from "ICMT")
	"ICMT", // Comments
	"ICOP", // Copyright
	"ICRD", // Creation Date
	"ICRP", // Cropped
	"IDIM", // Dimensions
	"IDPI", // Dots Per Inch
	"IENG", // Engineer
	"IGNR", // Genre
	"IKEY", // Keywords
	"ILGT", // Lightness
	"IMED", // Medium
	"INAM", // Name (title)
	"IPLT", // Palette Setting
	"IPRD", // Product (album)
	"ISBJ", // Subject
	"ISFT", // Source Software / Software
	"ISRC", // Source
	"ISRF", // Source Form
	"ITCH", // Technician
}

// fieldInfo describes an INFO field recognized by this package.
type fieldInfo struct {
	ID   string
	Name string
}

// knownFields maps every canonical INFO id to its human-readable name.
var knownFields = func() map[string]fieldInfo {
	names := map[string]string{
		"IARL": "Archival Location",
		"IART": "Artist",
		"ICMS": "Commissioned",
		"ICMP": "Compression Method",
		"ICMT": "Comments",
		"ICOP": "Copyright",
		"ICRD": "Creation Date",
		"ICRP": "Cropped",
		"IDIM": "Dimensions",
		"IDPI": "Dots Per Inch",
		"IENG": "Engineer",
		"IGNR": "Genre",
		"IKEY": "Keywords",
		"ILGT": "Lightness",
		"IMED": "Medium",
		"INAM": "Name",
		"IPLT": "Palette Setting",
		"IPRD": "Product",
		"ISBJ": "Subject",
		"ISFT": "Source Software",
		"ISRC": "Source",
		"ISRF": "Source Form",
		"ITCH": "Technician",
	}
	m := make(map[string]fieldInfo, len(names))
	for _, id := range canonicalOrder {
		m[id] = fieldInfo{ID: id, Name: names[id]}
	}
	return m
}()

// IsKnown reports whether id is one of the standard INFO field identifiers.
func IsKnown(id string) bool {
	_, ok := knownFields[id]
	return ok
}

// CanonicalOrder returns a copy of the authoritative order of standard INFO
// field identifiers.
func CanonicalOrder() []string {
	out := make([]string, len(canonicalOrder))
	copy(out, canonicalOrder)
	return out
}

// FieldName returns the documented name of a standard INFO field and true; for
// unknown identifiers it returns "" and false.
func FieldName(id string) (string, bool) {
	info, ok := knownFields[id]
	return info.Name, ok
}

// projectAliases maps external/project field names (lower-cased) onto INFO
// identifiers. Both the four-character INFO id and the descriptive names below
// are accepted wherever a project field key is resolved.
var projectAliases = map[string]string{
	// INFO ids (lower case) map to themselves.
	"iarl": "IARL", "iart": "IART", "icms": "ICMS", "icmp": "ICMP",
	"icmt": "ICMT", "icop": "ICOP", "icrd": "ICRD", "icrp": "ICRP",
	"idim": "IDIM", "idpi": "IDPI", "ieng": "IENG", "ignr": "IGNR",
	"ikey": "IKEY", "ilgt": "ILGT", "imed": "IMED", "inam": "INAM",
	"iplt": "IPLT", "iprd": "IPRD", "isbj": "ISBJ", "isft": "ISFT",
	"isrc": "ISRC", "isrf": "ISRF", "itch": "ITCH",

	// Common project / metadata vocabulary.
	"archival location": "IARL",
	"artist":            "IART",
	"commissioned":      "ICMS",
	"compression":       "ICMP",
	"comment":           "ICMT",
	"comments":          "ICMT",
	"copyright":         "ICOP",
	"date":              "ICRD",
	"created":           "ICRD",
	"creation date":     "ICRD",
	"engineer":          "IENG",
	"genre":             "IGNR",
	"keywords":          "IKEY",
	"medium":            "IMED",
	"title":             "INAM",
	"name":              "INAM",
	"album":             "IPRD",
	"product":           "IPRD",
	"subject":           "ISBJ",
	"software":          "ISFT",
	"source":            "ISRC",
	"technician":        "ITCH",
	"encoded by":        "ITCH",
}

// ResolveField maps a project field key (an INFO id, in any case, or a known
// descriptive/alias name) onto its canonical four-character INFO identifier.
// Keys that cannot be resolved are returned unchanged with ok == false, so
// callers can preserve them as bespoke fields instead of failing.
func ResolveField(key string) (id string, ok bool) {
	if len(key) == 4 {
		upper := toASCIIUpper(key)
		if IsKnown(upper) {
			return upper, true
		}
	}
	if id, hit := projectAliases[toASCIILower(key)]; hit {
		return id, true
	}
	return key, false
}

func toASCIILower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

func toASCIIUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}

// Package manifest defines the LTO hand-off inventory model, the tree
// scanner that produces it and the verifier used after re-arranging files.
package manifest

// Entry describes one regular file. Path is always root-relative and uses
// forward slashes so the manifest stays valid on every OS and after the
// directory is moved or re-arranged.
type Entry struct {
	Path      string `json:"path"`
	Class     string `json:"class"`
	MediaKind string `json:"mediaKind,omitempty"`
	MIMEType  string `json:"mime"`
	Ext       string `json:"ext"`
	Size      int64  `json:"size"`
	ModTime   string `json:"modTime"`
	SHA256    string `json:"sha256"`
}

// Relationship records a sidecar -> media dependency using both relative
// paths and content hashes, so the link survives a directory re-layout.
type Relationship struct {
	MediaPath   string `json:"mediaPath"`
	SidecarPath string `json:"sidecarPath"`
	MediaHash   string `json:"mediaHash"`
	SidecarHash string `json:"sidecarHash"`
	Rule        string `json:"rule"`
}

// DuplicateGroup lists identical-content files (same SHA-256).
type DuplicateGroup struct {
	SHA256 string   `json:"sha256"`
	Size   int64    `json:"size"`
	Paths  []string `json:"paths"`
	Wasted int64    `json:"wasted"`
}

// Issue is a structural problem found while scanning or verifying.
type Issue struct {
	Code   string `json:"code"`
	Path   string `json:"path,omitempty"`
	Detail string `json:"detail"`
}

// Budget summarizes footprint against the LTO capacity budget.
type Budget struct {
	Limit       int64   `json:"limit"`
	TotalSize   int64   `json:"totalSize"`
	UniqueSize  int64   `json:"uniqueSize"`
	Reclaimable int64   `json:"reclaimable"`
	Over        bool    `json:"over"`
	Ratio       float64 `json:"ratio"`
}

// Manifest is the top-level hand-off document. It never stores absolute paths.
type Manifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	GeneratedAt   string           `json:"generatedAt"`
	HashAlgorithm string           `json:"hashAlgorithm"`
	PathStyle     string           `json:"pathStyle"`
	Root          string           `json:"root"`
	EntryCount    int              `json:"entryCount"`
	TotalSize     int64            `json:"totalSize"`
	Entries       []Entry          `json:"entries"`
	Relationships []Relationship   `json:"relationships"`
	Duplicates    []DuplicateGroup `json:"duplicates"`
	Issues        []Issue          `json:"issues"`
	Budget        *Budget          `json:"budget,omitempty"`
}

// Package releaseaudit builds and validates the distribution matrix for a
// multi-version release package.
package releaseaudit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// SchemaVersion is the version of the JSON release package format understood
// by this package.
const SchemaVersion = 1

// Kind identifies a distribution version. The three kinds are intentionally
// closed: a release package may not silently introduce a fourth output while
// claiming to be complete.
type Kind string

const (
	KindTheatrical Kind = "theatrical"
	KindStreaming  Kind = "streaming"
	KindMobile     Kind = "mobile"
)

// DefaultKinds is the required version set for a complete release package.
var DefaultKinds = []Kind{KindTheatrical, KindStreaming, KindMobile}

// Package is the root release manifest. Paths are interpreted relative to the
// package root by the CLI, not relative to the manifest file itself.
type Package struct {
	SchemaVersion  int              `json:"schemaVersion"`
	Title          string           `json:"title,omitempty"`
	Versions       []Version        `json:"versions"`
	SharedManifest []SharedManifest `json:"sharedManifests,omitempty"`

	// RequiredKinds overrides DefaultKinds when non-empty. The field is
	// intentionally not part of the JSON wire format.
	RequiredKinds []Kind `json:"-"`
}

// Version describes one finished distribution version.
type Version struct {
	ID        string     `json:"id"`
	Label     string     `json:"label,omitempty"`
	Window    Window     `json:"window"`
	Video     Video      `json:"video"`
	Audio     Audio      `json:"audio"`
	Subtitles []Subtitle `json:"subtitles,omitempty"`
	Manifests []Manifest `json:"manifests,omitempty"`
}

// Window is the release window for a version.
type Window struct {
	Channel string `json:"channel,omitempty"`
	Start   string `json:"start,omitempty"`
	End     string `json:"end,omitempty"`
}

// Video describes the finished picture.
type Video struct {
	Path        string `json:"path"`
	AspectRatio string `json:"aspectRatio,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

// Audio describes the primary audio layout.
type Audio struct {
	Path     string `json:"path"`
	Layout   string `json:"layout,omitempty"`
	Channels int    `json:"channels,omitempty"`
	Language string `json:"language,omitempty"`
}

// Subtitle describes one subtitle or closed-caption track.
type Subtitle struct {
	Path     string `json:"path"`
	Language string `json:"language,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

// Manifest describes a manifest owned by exactly one version. Shared
// manifests must use Package.SharedManifest instead.
type Manifest struct {
	Path       string   `json:"path"`
	Kind       string   `json:"kind,omitempty"`
	References []string `json:"references,omitempty"`
}

// SharedManifest is a manifest file used by more than one version. Targets
// records the references each version expects the shared manifest to contain.
// An entry for every version is required.
type SharedManifest struct {
	Path    string           `json:"path"`
	Kind    string           `json:"kind,omitempty"`
	Targets []ManifestTarget `json:"targets"`
}

// ManifestTarget binds one version to the references it may see in a shared
// manifest.
type ManifestTarget struct {
	Version    string   `json:"version"`
	References []string `json:"references,omitempty"`
}

// Load decodes and structurally checks a release package. Asset existence and
// cross-version ownership are checked by Audit, because they need a package
// root.
func Load(r io.Reader) (*Package, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	var pkg Package
	if err := dec.Decode(&pkg); err != nil {
		return nil, fmt.Errorf("decode release manifest: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode release manifest: multiple JSON values")
		}
		return nil, fmt.Errorf("decode release manifest: %w", err)
	}
	if pkg.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported schemaVersion %d (want %d)", pkg.SchemaVersion, SchemaVersion)
	}
	if len(pkg.Versions) == 0 {
		return nil, fmt.Errorf("release manifest has no versions")
	}
	return &pkg, nil
}

// LoadFile reads a release package from path.
func LoadFile(path string) (*Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Load(f)
}

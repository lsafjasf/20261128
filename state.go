package mediaorient

import "fmt"

// Layer identifies a stage that may contribute a geometric orientation.
type Layer string

const (
	LayerPixels  Layer = "pixels"
	LayerDisplay Layer = "display-matrix"
	LayerPlayer  Layer = "player-transform"
)

// Source is one camera or file input. DisplayMatrix and PlayerTransform are
// metadata transforms; their zero value means identity.
type Source struct {
	ID               string      `json:"id"`
	Width            int         `json:"width"`
	Height           int         `json:"height"`
	PixelOrientation Orientation `json:"pixel_orientation"`
	DisplayMatrix    Matrix      `json:"display_matrix"`
	PlayerTransform  Matrix      `json:"player_transform"`
}

// LayerState records the decision made for one transform layer.
type LayerState struct {
	Layer       Layer       `json:"layer"`
	Declared    Orientation `json:"declared"`
	Applied     Orientation `json:"applied"`
	Duplicate   bool        `json:"duplicate"`
	DuplicateOf Layer       `json:"duplicate_of,omitempty"`
}

// Duplicate identifies a rotation that was present in more than one layer.
type Duplicate struct {
	Layer       Layer       `json:"layer"`
	KeptLayer   Layer       `json:"kept_layer"`
	Orientation Orientation `json:"orientation"`
}

// JointState is the single source of truth for raster direction, display
// metadata and player transforms. CanonicalMatrix maps stored raster pixels to
// upright pixels and includes every non-duplicate orientation exactly once.
type JointState struct {
	SourceID    string       `json:"source_id"`
	Raster      Size         `json:"raster"`
	Upright     Size         `json:"upright"`
	Orientation Orientation  `json:"orientation"`
	Canonical   Matrix       `json:"canonical_matrix"`
	Layers      []LayerState `json:"layers"`
	Duplicates  []Duplicate  `json:"duplicates,omitempty"`
}

// Resolve builds the joint state for a source. The earliest layer that
// declares a given orientation owns it; an identical orientation in a later
// layer is diagnostic evidence of duplicate rotation and is neutralized.
func (s Source) Resolve() (JointState, error) {
	if s.Width <= 0 || s.Height <= 0 {
		return JointState{}, fmt.Errorf("%w: source %q is %dx%d", ErrInvalidSize, s.ID, s.Width, s.Height)
	}
	if !s.PixelOrientation.Valid() {
		return JointState{}, fmt.Errorf(
			"%w: source %q has orientation %d",
			ErrInvalidOrientation, s.ID, s.PixelOrientation,
		)
	}

	display, err := s.DisplayMatrix.Normalize().Orientation(1e-7)
	if err != nil {
		return JointState{}, fmt.Errorf("source %q display matrix: %w", s.ID, err)
	}
	player, err := s.PlayerTransform.Normalize().Orientation(1e-7)
	if err != nil {
		return JointState{}, fmt.Errorf("source %q player transform: %w", s.ID, err)
	}

	declared := []LayerState{
		{Layer: LayerPixels, Declared: s.PixelOrientation},
		{Layer: LayerDisplay, Declared: display},
		{Layer: LayerPlayer, Declared: player},
	}
	canonical := OrientIdentity
	owner := map[Orientation]Layer{}
	var duplicates []Duplicate

	for i := range declared {
		current := declared[i]
		if current.Declared == OrientIdentity {
			current.Applied = OrientIdentity
			declared[i] = current
			continue
		}
		if keptBy, ok := owner[current.Declared]; ok {
			current.Duplicate = true
			current.DuplicateOf = keptBy
			current.Applied = OrientIdentity
			declared[i] = current
			duplicates = append(duplicates, Duplicate{
				Layer:       current.Layer,
				KeptLayer:   keptBy,
				Orientation: current.Declared,
			})
			continue
		}
		owner[current.Declared] = current.Layer
		canonical = canonical.Compose(current.Declared)
		current.Applied = current.Declared
		declared[i] = current
	}

	return JointState{
		SourceID:    s.ID,
		Raster:      Size{W: s.Width, H: s.Height},
		Upright:     canonical.Size(s.Width, s.Height),
		Orientation: canonical,
		Canonical:   canonical.Matrix(s.Width, s.Height),
		Layers:      declared,
		Duplicates:  duplicates,
	}, nil
}

// ResolveSource is the package-level form of Source.Resolve.
func ResolveSource(s Source) (JointState, error) { return s.Resolve() }

// DuplicateSummary explains duplicate rotations in a stable, human-readable
// form suitable for switch logs.
func (s JointState) DuplicateSummary() string {
	if len(s.Duplicates) == 0 {
		return fmt.Sprintf("source %s: no duplicate rotation", s.SourceID)
	}
	out := fmt.Sprintf("source %s:", s.SourceID)
	for _, d := range s.Duplicates {
		out += fmt.Sprintf(
			" %s repeats %s from %s; ignored",
			d.Layer, d.Orientation, d.KeptLayer,
		)
	}
	return out
}

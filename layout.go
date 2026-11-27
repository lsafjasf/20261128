package mediaorient

import (
	"fmt"
	"math"
)

// FitMode controls how upright source pixels are fitted into the output.
type FitMode uint8

const (
	FitContain FitMode = iota
	FitCover
	FitStretch
)

// OutputKind distinguishes interactive preview from the committed program
// frame. Both use the same joint state and the same layout engine.
type OutputKind string

const (
	OutputPreview OutputKind = "preview"
	OutputProgram OutputKind = "program"
)

// Insets are caption-safe margins in output pixels.
type Insets struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

// CaptionGeometry is deliberately expressed only in output/canvas space.
// Captions are composited after the source transform, so neither source
// rotation nor source cropping is applied to this rectangle a second time.
type CaptionGeometry struct {
	Canvas         Size   `json:"canvas"`
	Safe           Rect   `json:"safe"`
	NormalizedSafe Rect   `json:"normalized_safe"`
	WithinActive   bool   `json:"within_active"`
	Insets         Insets `json:"insets"`
}

// FrameGeometry is the resolved placement for one source in one output.
type FrameGeometry struct {
	Kind           OutputKind      `json:"kind"`
	Canvas         Size            `json:"canvas"`
	Upright        Size            `json:"upright"`
	ScaleX         float64         `json:"scale_x"`
	ScaleY         float64         `json:"scale_y"`
	Destination    Rect            `json:"destination"`
	SourceToCanvas Matrix          `json:"source_to_canvas"`
	Captions       CaptionGeometry `json:"captions"`
}

// Layout resolves a source and computes a complete preview or program frame
// geometry. Calling it with either OutputKind yields the same canonical
// orientation for the source; only resolution/fit differ.
func Layout(
	source Source,
	kind OutputKind,
	canvas Size,
	fit FitMode,
	insets Insets,
	captionsWithinActive bool,
) (JointState, FrameGeometry, error) {
	state, err := source.Resolve()
	if err != nil {
		return JointState{}, FrameGeometry{}, err
	}
	geometry, err := LayoutState(state, kind, canvas, fit, insets, captionsWithinActive)
	if err != nil {
		return JointState{}, FrameGeometry{}, err
	}
	return state, geometry, nil
}

// LayoutState computes output placement from an already resolved joint state.
func LayoutState(
	state JointState,
	kind OutputKind,
	canvas Size,
	fit FitMode,
	insets Insets,
	captionsWithinActive bool,
) (FrameGeometry, error) {
	if kind != OutputPreview && kind != OutputProgram {
		return FrameGeometry{}, fmt.Errorf("%w: output kind %q", ErrInvalidSession, kind)
	}
	if !canvas.Valid() {
		return FrameGeometry{}, fmt.Errorf("%w: canvas is %dx%d", ErrInvalidSize, canvas.W, canvas.H)
	}
	if !state.Upright.Valid() {
		return FrameGeometry{}, fmt.Errorf("%w: upright source is %dx%d", ErrInvalidSize, state.Upright.W, state.Upright.H)
	}
	if err := validateInsets(insets); err != nil {
		return FrameGeometry{}, err
	}

	uw := float64(state.Upright.W)
	uh := float64(state.Upright.H)
	cw := float64(canvas.W)
	ch := float64(canvas.H)
	sx, sy := cw/uw, ch/uh
	switch fit {
	case FitContain:
		sx = math.Min(sx, sy)
		sy = sx
	case FitCover:
		sx = math.Max(sx, sy)
		sy = sx
	case FitStretch:
	default:
		return FrameGeometry{}, fmt.Errorf("%w: fit mode %d", ErrInvalidSize, fit)
	}

	dw, dh := uw*sx, uh*sy
	destination := Rect{
		X: (cw - dw) / 2,
		Y: (ch - dh) / 2,
		W: dw,
		H: dh,
	}
	sourceToCanvas := state.Canonical.
		Then(Scale(sx, sy)).
		Then(Translate(destination.X, destination.Y))

	return FrameGeometry{
		Kind:           kind,
		Canvas:         canvas,
		Upright:        state.Upright,
		ScaleX:         sx,
		ScaleY:         sy,
		Destination:    destination,
		SourceToCanvas: sourceToCanvas,
		Captions:       captionGeometry(canvas, destination, insets, captionsWithinActive),
	}, nil
}

func captionGeometry(canvas Size, active Rect, insets Insets, withinActive bool) CaptionGeometry {
	cw, ch := float64(canvas.W), float64(canvas.H)
	safe := Rect{
		X: insets.Left,
		Y: insets.Top,
		W: math.Max(0, cw-insets.Left-insets.Right),
		H: math.Max(0, ch-insets.Top-insets.Bottom),
	}
	if withinActive {
		safe = safe.Intersect(active)
	}
	return CaptionGeometry{
		Canvas: canvas,
		Safe:   safe,
		NormalizedSafe: Rect{
			X: safe.X / cw,
			Y: safe.Y / ch,
			W: safe.W / cw,
			H: safe.H / ch,
		},
		WithinActive: withinActive,
		Insets:       insets,
	}
}

func validateInsets(insets Insets) error {
	values := []float64{insets.Left, insets.Top, insets.Right, insets.Bottom}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return fmt.Errorf("%w: invalid caption inset %g", ErrInvalidSize, value)
		}
	}
	return nil
}

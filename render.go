package mediaorient

import (
	"fmt"
	"image"
	"image/color"
	"math"
)

// RenderOptions describes one preview or program raster.
type RenderOptions struct {
	Kind                 OutputKind
	Canvas               Size
	Fit                  FitMode
	CaptionInsets        Insets
	CaptionsWithinActive bool
	Background           color.NRGBA
}

// Frame is a rendered raster together with the exact geometry used.
type Frame struct {
	Image    *image.NRGBA  `json:"-"`
	State    JointState    `json:"state"`
	Geometry FrameGeometry `json:"geometry"`
}

// Render applies the canonical orientation exactly once and fits the result
// into the requested output. Preview and program differ only by output canvas;
// both use the same joint state.
func (s Source) Render(raster image.Image, opts RenderOptions) (*Frame, error) {
	if raster == nil {
		return nil, fmt.Errorf("%w: raster is nil", ErrRasterSizeMismatch)
	}
	bounds := raster.Bounds()
	if bounds.Dx() != s.Width || bounds.Dy() != s.Height {
		return nil, fmt.Errorf(
			"%w: source %q is %dx%d, raster is %dx%d",
			ErrRasterSizeMismatch, s.ID, s.Width, s.Height,
			bounds.Dx(), bounds.Dy(),
		)
	}
	if opts.Kind == "" {
		opts.Kind = OutputProgram
	}
	if opts.Kind != OutputPreview && opts.Kind != OutputProgram {
		return nil, fmt.Errorf("%w: output kind %q", ErrInvalidSession, opts.Kind)
	}

	state, geometry, err := Layout(
		s, opts.Kind, opts.Canvas, opts.Fit,
		opts.CaptionInsets, opts.CaptionsWithinActive,
	)
	if err != nil {
		return nil, err
	}
	inverse, err := geometry.SourceToCanvas.Inverse()
	if err != nil {
		return nil, fmt.Errorf("invert source-to-canvas transform: %w", err)
	}

	dst := image.NewNRGBA(image.Rect(0, 0, opts.Canvas.W, opts.Canvas.H))
	fillNRGBA(dst, opts.Background)
	for y := 0; y < opts.Canvas.H; y++ {
		for x := 0; x < opts.Canvas.W; x++ {
			p := inverse.Apply(Point{X: float64(x) + 0.5, Y: float64(y) + 0.5})
			sx := int(math.Floor(p.X))
			sy := int(math.Floor(p.Y))
			if sx < 0 || sy < 0 || sx >= s.Width || sy >= s.Height {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBAModel.Convert(
				raster.At(bounds.Min.X+sx, bounds.Min.Y+sy),
			).(color.NRGBA))
		}
	}
	return &Frame{Image: dst, State: state, Geometry: geometry}, nil
}

func fillNRGBA(img *image.NRGBA, c color.NRGBA) {
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		row := img.Pix[img.PixOffset(img.Rect.Min.X, y):img.PixOffset(img.Rect.Max.X, y)]
		for x := 0; x < len(row); x += 4 {
			row[x], row[x+1], row[x+2], row[x+3] = c.R, c.G, c.B, c.A
		}
	}
}

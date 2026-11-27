// Package ghostdiag detects temporal denoiser trails in paired video frames.
// It uses only the Go standard library and analyzes the luma plane.
package ghostdiag

import (
	"fmt"
	"image"
	"image/color"
)

// Plane is an 8-bit luma image in row-major order.
type Plane struct {
	Width  int
	Height int
	Pix    []uint8
}

// Frame is one luma frame from the original or denoised sequence.
type Frame struct {
	Index     int
	Timestamp float64
	Plane     Plane
}

// Validate checks that a plane is internally consistent.
func (p Plane) Validate() error {
	if p.Width <= 0 || p.Height <= 0 {
		return fmt.Errorf("invalid frame dimensions %dx%d", p.Width, p.Height)
	}
	if len(p.Pix) != p.Width*p.Height {
		return fmt.Errorf("frame has %d samples, need %d", len(p.Pix), p.Width*p.Height)
	}
	return nil
}

// At returns the luma sample at x,y. The caller must keep coordinates in bounds.
func (p Plane) At(x, y int) uint8 {
	return p.Pix[y*p.Width+x]
}

// Clone returns an independent copy of the plane.
func (p Plane) Clone() Plane {
	return Plane{Width: p.Width, Height: p.Height, Pix: append([]uint8(nil), p.Pix...)}
}

// FromImage converts an image to an 8-bit luma plane using integer BT.601
// coefficients. Y is already used directly when the source is grayscale.
func FromImage(img image.Image) Plane {
	b := img.Bounds()
	p := Plane{Width: b.Dx(), Height: b.Dy(), Pix: make([]uint8, b.Dx()*b.Dy())}
	for y := 0; y < p.Height; y++ {
		for x := 0; x < p.Width; x++ {
			p.Pix[y*p.Width+x] = luma(img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return p
}

func luma(c color.Color) uint8 {
	r, g, b, _ := c.RGBA()
	return uint8((299*r + 587*g + 114*b + 500) / 1000 >> 8)
}

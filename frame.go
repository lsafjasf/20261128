package yuv

import (
	"fmt"
)

const (
	MinBitDepth = 8
	MaxBitDepth = 16
)

// Plane is one decoded, unclamped image plane. Pix contains code values in
// row-major order and may contain values outside the declared bit depth; that
// is intentional so an audit can distinguish bad input from conversion bugs.
type Plane struct {
	Name   string
	Width  int
	Height int
	Stride int
	Pix    []uint16
}

// Frame is a 4:2:0 frame in planar order: Y, Cb, Cr.
type Frame struct {
	Width           int
	Height          int
	BitDepth        int
	ChromaWidth     int
	ChromaHeight    int
	SampleFormat    SampleFormat
	PaddingAnomaly  int
	BitDepthAnomaly int
	Y               Plane
	Cb              Plane
	Cr              Plane
}

// SampleFormat describes how 10-bit codes are stored in a headerless file.
type SampleFormat string

const (
	// YUV420P10LE stores 10-bit codes in the low bits of little-endian uint16
	// words. This is FFmpeg's yuv420p10le layout.
	YUV420P10LE SampleFormat = "yuv420p10le"
	// P010LE stores 10-bit codes in the high bits of little-endian uint16
	// words. The low six padding bits must be zero.
	P010LE SampleFormat = "p010le"
)

func (f *Frame) Validate() error {
	if f.Width <= 0 || f.Height <= 0 {
		return fmt.Errorf("invalid frame dimensions %dx%d", f.Width, f.Height)
	}
	if f.BitDepth < MinBitDepth || f.BitDepth > MaxBitDepth {
		return fmt.Errorf("unsupported bit depth %d", f.BitDepth)
	}
	if f.ChromaWidth != (f.Width+1)/2 || f.ChromaHeight != (f.Height+1)/2 {
		return fmt.Errorf("invalid 4:2:0 chroma dimensions %dx%d for %dx%d",
			f.ChromaWidth, f.ChromaHeight, f.Width, f.Height)
	}
	for _, p := range []Plane{f.Y, f.Cb, f.Cr} {
		if p.Width <= 0 || p.Height <= 0 || p.Stride < p.Width {
			return fmt.Errorf("invalid plane %s geometry", p.Name)
		}
		if len(p.Pix) < p.Stride*p.Height {
			return fmt.Errorf("plane %s has %d samples, need %d",
				p.Name, len(p.Pix), p.Stride*p.Height)
		}
	}
	return nil
}

// Sample returns a code value using logical (visible) coordinates.
func (p Plane) Sample(x, y int) uint16 {
	return p.Pix[y*p.Stride+x]
}

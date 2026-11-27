package ghostdiag

import (
	"fmt"
	"io"
	"strings"
)

// RawConfig describes a headerless planar YUV stream.
type RawConfig struct {
	Width       int
	Height      int
	FrameRate   float64
	PixelFormat string
}

// RawSource reads 8-bit planar YUV, retaining only Y.
type RawSource struct {
	reader      io.Reader
	config      RawConfig
	index       int
	chromaBytes int
}

// NewRawSource validates a raw stream configuration.
func NewRawSource(r io.Reader, cfg RawConfig) (*RawSource, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("invalid raw dimensions %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.FrameRate <= 0 {
		cfg.FrameRate = 25
	}
	pix := strings.ToLower(cfg.PixelFormat)
	if pix == "" {
		pix = "yuv420p"
	}
	var chroma int
	switch pix {
	case "gray", "grey", "y":
		chroma = 0
	case "yuv420p", "yuvj420p":
		chroma = 2 * ((cfg.Width + 1) / 2) * ((cfg.Height + 1) / 2)
	case "yuv422p", "yuvj422p":
		chroma = 2 * ((cfg.Width + 1) / 2) * cfg.Height
	case "yuv444p", "yuvj444p":
		chroma = 2 * cfg.Width * cfg.Height
	default:
		return nil, fmt.Errorf("unsupported raw pixel format %q", cfg.PixelFormat)
	}
	cfg.PixelFormat = pix
	return &RawSource{reader: r, config: cfg, chromaBytes: chroma}, nil
}

func (s *RawSource) Format() Format {
	return Format{
		Width:       s.config.Width,
		Height:      s.config.Height,
		FrameRate:   s.config.FrameRate,
		PixelFormat: s.config.PixelFormat,
		Origin:      "raw",
	}
}

func (s *RawSource) Next() (Frame, error) {
	plane := make([]uint8, s.config.Width*s.config.Height)
	n, err := io.ReadFull(s.reader, plane)
	if err == io.EOF && n == 0 {
		return Frame{}, ErrEndOfStream
	}
	if err != nil {
		return Frame{}, fmt.Errorf("read raw frame %d luma: %w", s.index, err)
	}
	if s.chromaBytes > 0 {
		if _, err := io.CopyN(io.Discard, s.reader, int64(s.chromaBytes)); err != nil {
			return Frame{}, fmt.Errorf("read raw frame %d chroma: %w", s.index, err)
		}
	}
	frame := Frame{
		Index:     s.index,
		Timestamp: float64(s.index) / s.config.FrameRate,
		Plane: Plane{
			Width:  s.config.Width,
			Height: s.config.Height,
			Pix:    plane,
		},
	}
	s.index++
	return frame, nil
}

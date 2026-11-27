package video

import (
	"bufio"
	"fmt"
	"io"
)

// RawReader reads headerless planar 8-bit YUV. Only the luma plane is retained.
type RawReader struct {
	br         *bufio.Reader
	format     Format
	lumaBytes  int
	chromaSkip int
	index      int
}

func NewRawReader(r io.Reader, width, height int, fps float64, pixFmt string) (*RawReader, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("raw input requires a positive -size (for example 1920x1080)")
	}
	if fps <= 0 {
		fps = 25
	}
	format := Format{
		Width:       width,
		Height:      height,
		FrameRateN:  int(fps*1000 + 0.5),
		FrameRateD:  1000,
		Chroma:      pixFmt,
		Interlace:   "?",
		PixelFormat: pixFmt,
	}
	luma := width * height
	var total int
	switch pixFmt {
	case "gray":
		total = luma
	case "yuv420p", "420":
		cw, ch := (width+1)/2, (height+1)/2
		total = luma + 2*cw*ch
	case "yuv422p", "422":
		total = luma + 2*((width+1)/2)*height
	case "yuv444p", "444":
		total = luma + 2*width*height
	default:
		return nil, fmt.Errorf("unsupported raw pixel format %q", pixFmt)
	}
	return &RawReader{
		br:         bufio.NewReaderSize(r, 64*1024),
		format:     format,
		lumaBytes:  luma,
		chromaSkip: total - luma,
	}, nil
}

func (r *RawReader) Format() Format { return r.format }

func (r *RawReader) Next() (Frame, error) {
	y := make([]byte, r.lumaBytes)
	n, err := io.ReadFull(r.br, y)
	if err == io.EOF && n == 0 {
		return Frame{}, ErrEndOfStream
	}
	if err != nil {
		return Frame{}, fmt.Errorf("read raw frame %d luma: %w", r.index, err)
	}
	if r.chromaSkip > 0 {
		if _, err := io.CopyN(io.Discard, r.br, int64(r.chromaSkip)); err != nil {
			return Frame{}, fmt.Errorf("read raw frame %d chroma: %w", r.index, err)
		}
	}
	f := Frame{
		Index:       r.index,
		TimeSeconds: float64(r.index) / r.format.FrameRate(),
		Width:       r.format.Width,
		Height:      r.format.Height,
		Y:           y,
	}
	r.index++
	return f, nil
}

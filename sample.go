package yuv

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Layout describes a headerless 4:2:0 frame in a raw YUV file.
type Layout struct {
	Width        int
	Height       int
	BitDepth     int
	SampleFormat SampleFormat
	YStride      int
	CStride      int
}

func NewLayout(width, height, bitDepth int, format SampleFormat) Layout {
	return Layout{
		Width:        width,
		Height:       height,
		BitDepth:     bitDepth,
		SampleFormat: format,
		YStride:      width,
		CStride:      (width + 1) / 2,
	}
}

func (l Layout) Validate() error {
	if l.Width <= 0 || l.Height <= 0 {
		return fmt.Errorf("invalid dimensions %dx%d", l.Width, l.Height)
	}
	if l.Width > 1<<20 || l.Height > 1<<20 {
		return fmt.Errorf("unreasonable dimensions %dx%d", l.Width, l.Height)
	}
	if l.BitDepth != 10 {
		return fmt.Errorf("sample format %s requires 10-bit input, got %d",
			l.SampleFormat, l.BitDepth)
	}
	if l.SampleFormat != YUV420P10LE && l.SampleFormat != P010LE {
		return fmt.Errorf("unsupported sample format %q", l.SampleFormat)
	}
	if l.YStride < l.Width {
		return fmt.Errorf("Y stride %d is smaller than width %d", l.YStride, l.Width)
	}
	if l.CStride < (l.Width+1)/2 {
		return fmt.Errorf("chroma stride %d is smaller than chroma width %d",
			l.CStride, (l.Width+1)/2)
	}
	if l.YStride > 1<<20 || l.CStride > 1<<20 {
		return fmt.Errorf("unreasonable plane stride")
	}
	return nil
}

// FrameSize returns the exact byte size of one frame in a raw file.
func (l Layout) FrameSize() (int64, error) {
	if err := l.Validate(); err != nil {
		return 0, err
	}
	chromaHeight := (l.Height + 1) / 2
	yBytes := int64(l.YStride) * int64(l.Height) * 2
	cBytes := int64(l.CStride) * int64(chromaHeight) * 2
	return yBytes + 2*cBytes, nil
}

// ReadFrameAt reads one frame without clamping its code values.
func ReadFrameAt(r io.ReaderAt, offset int64, l Layout) (*Frame, error) {
	size, err := l.FrameSize()
	if err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, fmt.Errorf("negative frame offset %d", offset)
	}
	if size > int64(math.MaxInt) {
		return nil, fmt.Errorf("frame size %d exceeds addressable memory", size)
	}
	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, offset); err != nil {
		return nil, fmt.Errorf("read frame at %d: %w", offset, err)
	}

	chromaWidth := (l.Width + 1) / 2
	chromaHeight := (l.Height + 1) / 2
	yLen := l.YStride * l.Height
	cLen := l.CStride * chromaHeight
	if yLen > len(buf)/2 || 2*cLen > len(buf)/2-yLen {
		return nil, fmt.Errorf("frame size overflow")
	}
	words := make([]uint16, len(buf)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(buf[i*2:])
	}

	f := &Frame{
		Width:        l.Width,
		Height:       l.Height,
		BitDepth:     l.BitDepth,
		ChromaWidth:  chromaWidth,
		ChromaHeight: chromaHeight,
		SampleFormat: l.SampleFormat,
	}
	f.Y = Plane{Name: "Y", Width: l.Width, Height: l.Height,
		Stride: l.YStride, Pix: words[:yLen]}
	f.Cb = Plane{Name: "Cb", Width: chromaWidth, Height: chromaHeight,
		Stride: l.CStride, Pix: words[yLen : yLen+cLen]}
	f.Cr = Plane{Name: "Cr", Width: chromaWidth, Height: chromaHeight,
		Stride: l.CStride, Pix: words[yLen+cLen : yLen+2*cLen]}
	f.normalizeSamples()
	return f, nil
}

func (f *Frame) normalizeSamples() {
	if f.SampleFormat == P010LE {
		for _, p := range []*Plane{&f.Y, &f.Cb, &f.Cr} {
			for y := 0; y < p.Height; y++ {
				row := p.Pix[y*p.Stride : y*p.Stride+p.Width]
				for x, word := range row {
					if word&0x3f != 0 {
						f.PaddingAnomaly++
					}
					row[x] = word >> 6
				}
			}
		}
		return
	}
	max := uint16((1 << f.BitDepth) - 1)
	for _, p := range []*Plane{&f.Y, &f.Cb, &f.Cr} {
		for y := 0; y < p.Height; y++ {
			row := p.Pix[y*p.Stride : y*p.Stride+p.Width]
			for _, code := range row {
				if code > max {
					f.BitDepthAnomaly++
				}
			}
		}
	}
}

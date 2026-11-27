package ghostdiag

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Y4MSource reads 8-bit planar Y4M. Only the luma plane is retained.
type Y4MSource struct {
	reader      *bufio.Reader
	format      Format
	index       int
	chromaBytes int
}

// NewY4MSource parses a Y4M header and prepares the source for reading.
func NewY4MSource(r io.Reader) (*Y4MSource, error) {
	br := bufio.NewReader(r)
	header, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read Y4M header: %w", err)
	}
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "YUV4MPEG2 ") {
		return nil, fmt.Errorf("invalid Y4M header")
	}
	format := Format{FrameRate: 25, PixelFormat: "420", Origin: "y4m"}
	for _, field := range strings.Fields(header) {
		if len(field) < 2 {
			continue
		}
		value := field[1:]
		switch field[0] {
		case 'W':
			format.Width, _ = strconv.Atoi(value)
		case 'H':
			format.Height, _ = strconv.Atoi(value)
		case 'F':
			format.FrameRate = parseFrameRate(value)
		case 'C':
			format.PixelFormat = strings.ToLower(value)
		}
	}
	if format.Width <= 0 || format.Height <= 0 {
		return nil, fmt.Errorf("Y4M header is missing valid W/H")
	}
	chroma, err := y4mChromaBytes(format.PixelFormat, format.Width, format.Height)
	if err != nil {
		return nil, err
	}
	return &Y4MSource{reader: br, format: format, chromaBytes: chroma}, nil
}

func parseFrameRate(value string) float64 {
	parts := strings.SplitN(value, ":", 2)
	num, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || num <= 0 {
		return 25
	}
	if len(parts) == 1 {
		return num
	}
	den, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || den <= 0 {
		return 25
	}
	return num / den
}

func y4mChromaBytes(pixelFormat string, width, height int) (int, error) {
	switch {
	case pixelFormat == "mono":
		return 0, nil
	case strings.Contains(pixelFormat, "p10"), strings.Contains(pixelFormat, "p12"),
		strings.Contains(pixelFormat, "p16"):
		return 0, fmt.Errorf("Y4M pixel format %q is not 8-bit", pixelFormat)
	case strings.HasPrefix(pixelFormat, "420"):
		return 2 * ((width + 1) / 2) * ((height + 1) / 2), nil
	case strings.HasPrefix(pixelFormat, "422"):
		return 2 * ((width + 1) / 2) * height, nil
	case strings.HasPrefix(pixelFormat, "444"):
		return 2 * width * height, nil
	default:
		return 0, fmt.Errorf("unsupported Y4M pixel format %q", pixelFormat)
	}
}

func (s *Y4MSource) Format() Format {
	return s.format
}

func (s *Y4MSource) Next() (Frame, error) {
	frameHeader, err := s.reader.ReadString('\n')
	if err == io.EOF && frameHeader == "" {
		return Frame{}, ErrEndOfStream
	}
	if err != nil {
		return Frame{}, fmt.Errorf("read Y4M frame %d header: %w", s.index, err)
	}
	if !strings.HasPrefix(strings.TrimSpace(frameHeader), "FRAME") {
		return Frame{}, fmt.Errorf("Y4M frame %d has invalid header", s.index)
	}
	plane := make([]uint8, s.format.Width*s.format.Height)
	if _, err := io.ReadFull(s.reader, plane); err != nil {
		return Frame{}, fmt.Errorf("read Y4M frame %d luma: %w", s.index, err)
	}
	if s.chromaBytes > 0 {
		if _, err := io.CopyN(io.Discard, s.reader, int64(s.chromaBytes)); err != nil {
			return Frame{}, fmt.Errorf("read Y4M frame %d chroma: %w", s.index, err)
		}
	}
	frame := Frame{
		Index:     s.index,
		Timestamp: float64(s.index) / s.format.FrameRate,
		Plane: Plane{
			Width:  s.format.Width,
			Height: s.format.Height,
			Pix:    plane,
		},
	}
	s.index++
	return frame, nil
}

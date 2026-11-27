package video

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Y4MReader struct {
	br         *bufio.Reader
	format     Format
	frameBytes int
	lumaBytes  int
	index      int
}

func NewY4MReader(r io.Reader) (*Y4MReader, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read y4m header: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 || fields[0] != "YUV4MPEG2" {
		return nil, fmt.Errorf("not a y4m stream")
	}

	f := Format{FrameRateN: 25, FrameRateD: 1, Chroma: "420jpeg", Interlace: "?", PixelFormat: "yuv420p"}
	for _, field := range fields[1:] {
		if len(field) < 2 {
			continue
		}
		value := field[1:]
		switch field[0] {
		case 'W':
			f.Width, _ = strconv.Atoi(value)
		case 'H':
			f.Height, _ = strconv.Atoi(value)
		case 'F':
			parts := strings.SplitN(value, ":", 2)
			if len(parts) == 2 {
				f.FrameRateN, _ = strconv.Atoi(parts[0])
				f.FrameRateD, _ = strconv.Atoi(parts[1])
			}
		case 'C':
			f.Chroma = strings.ToLower(value)
		case 'I':
			f.Interlace = strings.ToLower(value)
		}
	}
	if f.Width <= 0 || f.Height <= 0 {
		return nil, fmt.Errorf("invalid y4m dimensions %dx%d", f.Width, f.Height)
	}
	if f.FrameRateN <= 0 || f.FrameRateD <= 0 {
		return nil, fmt.Errorf("invalid y4m frame rate")
	}
	luma, total, err := planeBytes(f.Width, f.Height, f.Chroma)
	if err != nil {
		return nil, err
	}
	return &Y4MReader{br: br, format: f, frameBytes: total, lumaBytes: luma}, nil
}

func planeBytes(w, h int, chroma string) (luma, total int, err error) {
	luma = w * h
	cw, ch := (w+1)/2, (h+1)/2
	switch chroma {
	case "mono":
		return luma, luma, nil
	case "420", "420jpeg", "420mpeg2", "420paldv":
		return luma, luma + 2*cw*ch, nil
	case "422":
		return luma, luma + 2*cw*h, nil
	case "444":
		return luma, luma + 2*w*h, nil
	default:
		return 0, 0, fmt.Errorf("unsupported y4m chroma format %q (8-bit 4:2:0/4:2:2/4:4:4/mono only)", chroma)
	}
}

func (r *Y4MReader) Format() Format { return r.format }

func (r *Y4MReader) Next() (Frame, error) {
	line, err := r.br.ReadString('\n')
	if err == io.EOF && line == "" {
		return Frame{}, ErrEndOfStream
	}
	if err != nil {
		return Frame{}, fmt.Errorf("read y4m frame header: %w", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(line), "FRAME") {
		return Frame{}, fmt.Errorf("expected FRAME header, got %q", strings.TrimSpace(line))
	}
	buf := make([]byte, r.frameBytes)
	if _, err := io.ReadFull(r.br, buf); err != nil {
		return Frame{}, fmt.Errorf("read y4m frame %d: %w", r.index, err)
	}
	f := Frame{
		Index:       r.index,
		TimeSeconds: float64(r.index) / r.format.FrameRate(),
		Width:       r.format.Width,
		Height:      r.format.Height,
		Y:           buf[:r.lumaBytes],
	}
	r.index++
	return f, nil
}

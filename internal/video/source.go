package video

import "errors"

var ErrEndOfStream = errors.New("end of stream")

// Format describes enough of the input to interpret luma samples. Chroma is
// consumed but deliberately not analysed: combing, sharpness and edge motion
// are all evaluated on the luma plane.
type Format struct {
	Width       int
	Height      int
	FrameRateN  int
	FrameRateD  int
	Chroma      string
	Interlace   string
	PixelFormat string
}

func (f Format) FrameRate() float64 {
	if f.FrameRateN <= 0 || f.FrameRateD <= 0 {
		return 0
	}
	return float64(f.FrameRateN) / float64(f.FrameRateD)
}

// Frame owns its luma buffer. Implementations should use a new buffer per
// frame so an analyser can retain the previous frame without copying it.
type Frame struct {
	Index       int
	TimeSeconds float64
	Width       int
	Height      int
	Y           []byte
}

type Source interface {
	Format() Format
	Next() (Frame, error)
}

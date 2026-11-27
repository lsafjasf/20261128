package ghostdiag

import "errors"

// ErrEndOfStream is returned by Source.Next at a clean sequence boundary.
var ErrEndOfStream = errors.New("end of frame stream")

// Format describes the decoded frame geometry used for pair validation.
type Format struct {
	Width       int
	Height      int
	FrameRate   float64
	PixelFormat string
	Origin      string
}

// Source yields luma frames in presentation order.
type Source interface {
	Next() (Frame, error)
	Format() Format
}

// CloseSource closes a source when its concrete type owns resources.
func CloseSource(source Source) error {
	if closer, ok := source.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

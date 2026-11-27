package mediaorient

import "errors"

var (
	ErrInvalidOrientation = errors.New("invalid orientation")
	ErrInvalidSize        = errors.New("invalid size")
	ErrUnsupportedMatrix  = errors.New("matrix is not an axis-aligned rigid transform")
	ErrSourceNotFound     = errors.New("source not found")
	ErrInvalidSession     = errors.New("invalid session")
	ErrRasterSizeMismatch = errors.New("raster size does not match source")
)

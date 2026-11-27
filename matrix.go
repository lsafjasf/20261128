package yuv

import (
	"fmt"
	"math"
)

// Matrix identifies the YCbCr matrix. The aliases match common ffprobe names.
type Matrix string

const (
	MatrixBT601  Matrix = "bt601"
	MatrixBT709  Matrix = "bt709"
	MatrixBT2020 Matrix = "bt2020"
)

func ParseMatrix(s string) (Matrix, error) {
	switch Matrix(s) {
	case MatrixBT601, "bt470bg", "smpte170m":
		return MatrixBT601, nil
	case MatrixBT709:
		return MatrixBT709, nil
	case MatrixBT2020, "bt2020nc", "bt2020ncl":
		return MatrixBT2020, nil
	default:
		return "", fmt.Errorf("unknown matrix %q (want bt601, bt709 or bt2020)", s)
	}
}

func (m Matrix) krkb() (kr, kb float64, err error) {
	switch m {
	case MatrixBT601:
		return 0.299, 0.114, nil
	case MatrixBT709:
		return 0.2126, 0.0722, nil
	case MatrixBT2020:
		return 0.2627, 0.0593, nil
	default:
		return 0, 0, fmt.Errorf("unknown matrix %q", m)
	}
}

// RGBFromNormalized applies the non-constant-luminance YCbCr matrix. y is in
// [0,1]; cb and cr are centered chroma in [-0.5,0.5].
func (m Matrix) RGBFromNormalized(y, cb, cr float64) (r, g, b float64, err error) {
	kr, kb, err := m.krkb()
	if err != nil {
		return 0, 0, 0, err
	}
	kg := 1 - kr - kb
	r = y + 2*(1-kr)*cr
	b = y + 2*(1-kb)*cb
	g = y - (2*kb*(1-kb)/kg)*cb - (2*kr*(1-kr)/kg)*cr
	return r, g, b, nil
}

// NormalizeCodes converts integer 4:2:0 codes to normalized components. It
// deliberately applies no clipping; an invalid code remains visible to the
// caller as an out-of-range normalized component.
func NormalizeCodes(y, cb, cr, bitDepth int, r Range) (yn, cbn, crn float64, err error) {
	yLo, yHi, cLo, cHi, err := r.Bounds(bitDepth)
	if err != nil {
		return 0, 0, 0, err
	}
	mid := 1 << (bitDepth - 1)
	if r == Limited {
		yn = float64(y-yLo) / float64(yHi-yLo)
		cbn = float64(cb-mid) / float64(cHi-cLo)
		crn = float64(cr-mid) / float64(cHi-cLo)
		return yn, cbn, crn, nil
	}
	maxInt := (1 << bitDepth) - 1
	max := float64(maxInt)
	yn = float64(y) / max
	cbn = float64(cb-mid) / max
	crn = float64(cr-mid) / max
	return yn, cbn, crn, nil
}

// QuantizeRGB rounds to nearest integer and clips only at the display
// boundary. Out reports whether nearest rounding still produced a code outside
// the display range, which lets callers count real clipping instead of hiding
// it.
func QuantizeRGB(v float64, bitDepth int) (code int, out bool) {
	max := (1 << bitDepth) - 1
	return quantizeCode(v, max)
}

func quantizeCode(v float64, max int) (code int, out bool) {
	code = int(math.Round(v))
	if code < 0 {
		return 0, true
	}
	if code > max {
		return max, true
	}
	return code, false
}

package mediaorient

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// Orientation is one of the eight axis-aligned orientations in the dihedral
// group D4. It describes the rotation needed to turn stored pixels upright.
// A mirrored orientation is represented as a clockwise rotation followed by a
// horizontal flip.
type Orientation uint8

const (
	OrientIdentity Orientation = iota
	OrientFlipH
	OrientRotate180
	OrientFlipV
	OrientTranspose
	OrientRotate90
	OrientTransverse
	OrientRotate270
)

// Rotation is a clockwise display rotation in degrees.
type Rotation uint16

const (
	Rotate0 Rotation = iota * 90
	Rotate90
	Rotate180
	Rotate270
)

// OrientationFromEXIF converts EXIF orientation values 1..8 to Orientation.
func OrientationFromEXIF(exif int) (Orientation, error) {
	if exif < 1 || exif > 8 {
		return OrientIdentity, fmt.Errorf("%w: EXIF value %d", ErrInvalidOrientation, exif)
	}
	return Orientation(exif - 1), nil
}

// EXIF returns the equivalent EXIF orientation value in 1..8.
func (o Orientation) EXIF() int {
	if !o.Valid() {
		return 0
	}
	return int(o) + 1
}

// MarshalJSON emits the standard EXIF orientation number (1..8).
func (o Orientation) MarshalJSON() ([]byte, error) {
	if !o.Valid() {
		return nil, fmt.Errorf("%w: cannot encode orientation %d", ErrInvalidOrientation, o)
	}
	return []byte(strconv.Itoa(o.EXIF())), nil
}

// UnmarshalJSON accepts a standard EXIF orientation number (1..8).
func (o *Orientation) UnmarshalJSON(data []byte) error {
	var exif int
	if err := json.Unmarshal(data, &exif); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidOrientation, err)
	}
	parsed, err := OrientationFromEXIF(exif)
	if err != nil {
		return err
	}
	*o = parsed
	return nil
}

// Valid reports whether o is one of the eight D4 orientations.
func (o Orientation) Valid() bool { return o <= OrientRotate270 }

// Rotation returns the clockwise rotation component of o.
func (o Orientation) Rotation() Rotation {
	switch o {
	case OrientRotate90, OrientTranspose:
		return Rotate90
	case OrientRotate180, OrientFlipV:
		return Rotate180
	case OrientRotate270, OrientTransverse:
		return Rotate270
	default:
		return Rotate0
	}
}

// Mirrored reports whether a horizontal mirror is applied after Rotation.
func (o Orientation) Mirrored() bool {
	switch o {
	case OrientFlipH, OrientFlipV, OrientTranspose, OrientTransverse:
		return true
	default:
		return false
	}
}

// SwapsAxes reports whether o exchanges width and height.
func (o Orientation) SwapsAxes() bool {
	switch o {
	case OrientTranspose, OrientRotate90, OrientTransverse, OrientRotate270:
		return true
	default:
		return false
	}
}

// Size returns the upright dimensions of a w by h stored raster.
func (o Orientation) Size(w, h int) Size {
	if o.SwapsAxes() {
		return Size{W: h, H: w}
	}
	return Size{W: w, H: h}
}

// Matrix returns the normalized raster-to-upright transform for a w by h
// raster. Pixels occupy the continuous range [0,w] x [0,h].
func (o Orientation) Matrix(w, h int) Matrix {
	fw, fh := float64(w), float64(h)
	switch o {
	case OrientFlipH:
		return Matrix{A: -1, D: 1, TX: fw}
	case OrientRotate180:
		return Matrix{A: -1, D: -1, TX: fw, TY: fh}
	case OrientFlipV:
		return Matrix{A: 1, D: -1, TY: fh}
	case OrientTranspose:
		return Matrix{B: 1, C: 1}
	case OrientRotate90:
		return Matrix{B: -1, C: 1, TX: fh}
	case OrientTransverse:
		return Matrix{B: -1, C: -1, TX: fh, TY: fw}
	case OrientRotate270:
		return Matrix{B: 1, C: -1, TY: fw}
	default:
		return IdentityMatrix()
	}
}

// Compose applies o first and next second.
func (o Orientation) Compose(next Orientation) Orientation {
	if !o.Valid() || !next.Valid() {
		return OrientIdentity
	}
	m := o.Matrix(1, 1).Then(next.Matrix(1, 1))
	res, err := orientationFromLinear(m, 1e-9)
	if err != nil {
		return OrientIdentity
	}
	return res
}

// String returns a stable diagnostic name.
func (o Orientation) String() string {
	switch o {
	case OrientIdentity:
		return "identity"
	case OrientFlipH:
		return "flip-horizontal"
	case OrientRotate180:
		return "rotate-180"
	case OrientFlipV:
		return "flip-vertical"
	case OrientTranspose:
		return "transpose"
	case OrientRotate90:
		return "rotate-90-cw"
	case OrientTransverse:
		return "transverse"
	case OrientRotate270:
		return "rotate-270-cw"
	default:
		return fmt.Sprintf("orientation(%d)", uint8(o))
	}
}

func orientationFromLinear(m Matrix, epsilon float64) (Orientation, error) {
	if epsilon <= 0 {
		epsilon = 1e-9
	}
	for o := OrientIdentity; o <= OrientRotate270; o++ {
		want := o.Matrix(1, 1)
		if math.Abs(m.A-want.A) <= epsilon &&
			math.Abs(m.B-want.B) <= epsilon &&
			math.Abs(m.C-want.C) <= epsilon &&
			math.Abs(m.D-want.D) <= epsilon {
			return o, nil
		}
	}
	return OrientIdentity, ErrUnsupportedMatrix
}

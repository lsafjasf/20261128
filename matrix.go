package mediaorient

import (
	"fmt"
	"math"
)

// Size is an integer extent.
type Size struct {
	W int `json:"w"`
	H int `json:"h"`
}

// Valid reports whether both dimensions are positive.
func (s Size) Valid() bool { return s.W > 0 && s.H > 0 }

// Point is a position in continuous pixel coordinates.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Rect is an axis-aligned rectangle in continuous pixel coordinates.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Size returns the rectangle's extent.
func (r Rect) Size() Size {
	return Size{W: int(math.Round(r.W)), H: int(math.Round(r.H))}
}

// Empty reports whether the rectangle has no positive area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Intersect returns the overlap between r and other.
func (r Rect) Intersect(other Rect) Rect {
	x0 := math.Max(r.X, other.X)
	y0 := math.Max(r.Y, other.Y)
	x1 := math.Min(r.X+r.W, other.X+other.W)
	y1 := math.Min(r.Y+r.H, other.Y+other.H)
	if x1 <= x0 || y1 <= y0 {
		return Rect{X: x0, Y: y0}
	}
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Matrix is a 2D affine transform:
//
//	x' = A*x + B*y + TX
//	y' = C*x + D*y + TY
//
// The zero value is treated as identity when supplied as display or player
// metadata. This keeps omitted transforms ergonomic and unambiguous.
type Matrix struct {
	A  float64 `json:"a"`
	B  float64 `json:"b"`
	C  float64 `json:"c"`
	D  float64 `json:"d"`
	TX float64 `json:"tx"`
	TY float64 `json:"ty"`
}

// IdentityMatrix returns the identity transform.
func IdentityMatrix() Matrix { return Matrix{A: 1, D: 1} }

// Matrix3x3 builds an affine transform from row-major 3x3 metadata.
func Matrix3x3(v [9]float64) (Matrix, error) {
	if !allFinite(v[:]...) {
		return Matrix{}, fmt.Errorf("%w: non-finite matrix value", ErrUnsupportedMatrix)
	}
	if !nearlyEqual(v[6], 0, 1e-9) ||
		!nearlyEqual(v[7], 0, 1e-9) ||
		!nearlyEqual(v[8], 1, 1e-9) {
		return Matrix{}, fmt.Errorf("%w: bottom row must be [0 0 1]", ErrUnsupportedMatrix)
	}
	return Matrix{A: v[0], B: v[1], C: v[3], D: v[4], TX: v[2], TY: v[5]}, nil
}

// IsZero reports whether every matrix coefficient is zero.
func (m Matrix) IsZero() bool {
	return m == Matrix{}
}

// Normalize maps the zero value to identity and leaves other matrices intact.
func (m Matrix) Normalize() Matrix {
	if m.IsZero() {
		return IdentityMatrix()
	}
	return m
}

// Then returns the transform that applies m first and next second.
func (m Matrix) Then(next Matrix) Matrix {
	return next.Mul(m)
}

// Mul returns the conventional matrix product m * rhs.
func (m Matrix) Mul(rhs Matrix) Matrix {
	return Matrix{
		A:  m.A*rhs.A + m.B*rhs.C,
		B:  m.A*rhs.B + m.B*rhs.D,
		C:  m.C*rhs.A + m.D*rhs.C,
		D:  m.C*rhs.B + m.D*rhs.D,
		TX: m.A*rhs.TX + m.B*rhs.TY + m.TX,
		TY: m.C*rhs.TX + m.D*rhs.TY + m.TY,
	}
}

// Apply maps p through m.
func (m Matrix) Apply(p Point) Point {
	m = m.Normalize()
	return Point{
		X: m.A*p.X + m.B*p.Y + m.TX,
		Y: m.C*p.X + m.D*p.Y + m.TY,
	}
}

// Orientation extracts the D4 orientation from the matrix's linear part.
// Translation is intentionally ignored because it controls placement, not
// direction.
func (m Matrix) Orientation(epsilon float64) (Orientation, error) {
	m = m.Normalize()
	if epsilon <= 0 {
		epsilon = 1e-7
	}
	if !allFinite(m.A, m.B, m.C, m.D, m.TX, m.TY) {
		return OrientIdentity, fmt.Errorf("%w: non-finite coefficient", ErrUnsupportedMatrix)
	}

	col1 := math.Hypot(m.A, m.C)
	col2 := math.Hypot(m.B, m.D)
	if !nearlyEqual(col1, 1, epsilon) || !nearlyEqual(col2, 1, epsilon) {
		return OrientIdentity, fmt.Errorf("%w: scale %g/%g", ErrUnsupportedMatrix, col1, col2)
	}
	if !nearlyEqual(m.A*m.B+m.C*m.D, 0, epsilon) {
		return OrientIdentity, fmt.Errorf("%w: skewed axes", ErrUnsupportedMatrix)
	}
	return orientationFromLinear(m, epsilon)
}

// Inverse returns the inverse affine transform.
func (m Matrix) Inverse() (Matrix, error) {
	m = m.Normalize()
	det := m.A*m.D - m.B*m.C
	if math.Abs(det) < 1e-12 || !allFinite(m.A, m.B, m.C, m.D, m.TX, m.TY) {
		return Matrix{}, fmt.Errorf("%w: non-invertible matrix", ErrUnsupportedMatrix)
	}
	ia, ib := m.D/det, -m.B/det
	ic, id := -m.C/det, m.A/det
	return Matrix{
		A:  ia,
		B:  ib,
		C:  ic,
		D:  id,
		TX: -(ia*m.TX + ib*m.TY),
		TY: -(ic*m.TX + id*m.TY),
	}, nil
}

// Translate returns a translation transform.
func Translate(x, y float64) Matrix {
	return Matrix{A: 1, D: 1, TX: x, TY: y}
}

// Scale returns a non-uniform scale transform.
func Scale(x, y float64) Matrix {
	return Matrix{A: x, D: y}
}

func allFinite(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func nearlyEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) <= epsilon
}

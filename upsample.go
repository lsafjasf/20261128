package yuv

import "fmt"

// ChromaLocation describes the position of a chroma sample relative to the
// luma grid. The horizontal and vertical phases are in units of a luma sample.
type ChromaLocation string

const (
	ChromaLeft    ChromaLocation = "left"
	ChromaCenter  ChromaLocation = "center"
	ChromaTop     ChromaLocation = "top"
	ChromaTopLeft ChromaLocation = "topleft"
)

func ParseChromaLocation(s string) (ChromaLocation, error) {
	switch ChromaLocation(s) {
	case ChromaLeft:
		return ChromaLeft, nil
	case ChromaCenter, "jpeg":
		return ChromaCenter, nil
	case ChromaTop:
		return ChromaTop, nil
	case ChromaTopLeft:
		return ChromaTopLeft, nil
	default:
		return "", fmt.Errorf("unknown chroma location %q (want left, center, top or topleft)", s)
	}
}

// quarterPhase returns the chroma sample phase in quarter-luma units.
func (c ChromaLocation) quarterPhase() (x, y int, err error) {
	switch c {
	case ChromaLeft:
		return 0, 2, nil
	case ChromaCenter:
		return 2, 2, nil
	case ChromaTop:
		return 2, 0, nil
	case ChromaTopLeft:
		return 0, 0, nil
	default:
		return 0, 0, fmt.Errorf("unknown chroma location %q", c)
	}
}

type tap struct {
	i0, i1 int
	w0, w1 int
}

func makeTaps(size, chromaSize, phaseQuarter int) []tap {
	taps := make([]tap, size)
	for p := range taps {
		// c = (p - phase) / 2. Multiplying numerator and denominator by four
		// gives num=(4p-phaseQuarter); i0=floor(num/8), f=rem/8.
		num := 4*p - phaseQuarter
		i0 := floorDiv8(num)
		rem := num - 8*i0
		if i0 < 0 {
			taps[p] = tap{i0: 0, i1: 0, w0: 8}
			continue
		}
		if i0 >= chromaSize-1 {
			if i0 >= chromaSize {
				i0 = chromaSize - 1
			}
			taps[p] = tap{i0: i0, i1: i0, w0: 8}
			continue
		}
		taps[p] = tap{i0: i0, i1: i0 + 1, w0: 8 - rem, w1: rem}
	}
	return taps
}

func floorDiv8(v int) int {
	if v >= 0 {
		return v / 8
	}
	return -((-v + 7) / 8)
}

// upsamplePlane performs edge-clamped bilinear interpolation using integer
// weights. Its weights are convex, so the result cannot overshoot the source
// chroma samples.
func upsamplePlane(p Plane, width, height int, loc ChromaLocation) ([]uint16, error) {
	px, py, err := loc.quarterPhase()
	if err != nil {
		return nil, err
	}
	xt := makeTaps(width, p.Width, px)
	yt := makeTaps(height, p.Height, py)
	out := make([]uint16, width*height)
	for y := 0; y < height; y++ {
		ty := yt[y]
		for x := 0; x < width; x++ {
			tx := xt[x]
			s00 := int(p.Sample(tx.i0, ty.i0))
			s01 := int(p.Sample(tx.i1, ty.i0))
			s10 := int(p.Sample(tx.i0, ty.i1))
			s11 := int(p.Sample(tx.i1, ty.i1))
			sum := s00*tx.w0*ty.w0 + s01*tx.w1*ty.w0 +
				s10*tx.w0*ty.w1 + s11*tx.w1*ty.w1
			out[y*width+x] = uint16((sum + 32) >> 6)
		}
	}
	return out, nil
}

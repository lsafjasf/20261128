package yuv

import "fmt"

// Range is the quantization range declared by the source/container.
type Range string

const (
	Limited Range = "limited"
	Full    Range = "full"
)

func ParseRange(s string) (Range, error) {
	switch Range(s) {
	case Limited, "tv", "video":
		return Limited, nil
	case Full, "pc", "jpeg":
		return Full, nil
	default:
		return "", fmt.Errorf("unknown range %q (want limited or full)", s)
	}
}

func (r Range) String() string {
	return string(r)
}

// Bounds returns the nominal legal code range. Limited range is scaled from
// the 8-bit values defined by BT.601/709: Y 16..235 and chroma 16..240.
func (r Range) Bounds(bitDepth int) (yLo, yHi, cLo, cHi int, err error) {
	if bitDepth < MinBitDepth || bitDepth > MaxBitDepth {
		return 0, 0, 0, 0, fmt.Errorf("unsupported bit depth %d", bitDepth)
	}
	max := (1 << bitDepth) - 1
	switch r {
	case Limited:
		shift := bitDepth - 8
		return 16 << shift, 235 << shift, 16 << shift, 240 << shift, nil
	case Full:
		return 0, max, 0, max, nil
	default:
		return 0, 0, 0, 0, fmt.Errorf("unknown range %q", r)
	}
}

func (r Range) MaxCode(bitDepth int) int {
	return (1 << bitDepth) - 1
}

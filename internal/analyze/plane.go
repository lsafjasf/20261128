package analyze

import (
	"math"

	"interlace-diag/internal/video"
)

type plane struct {
	W     int
	H     int
	Pix   []byte
	profX []float64
	profY []float64
}

func extractField(f video.Frame, parity int) plane {
	h := (f.Height - parity + 1) / 2
	p := plane{W: f.Width, H: h, Pix: make([]byte, f.Width*h)}
	for y := 0; y < h; y++ {
		src := (2*y + parity) * f.Width
		copy(p.Pix[y*f.Width:(y+1)*f.Width], f.Y[src:src+f.Width])
	}
	return p
}

func (p plane) at(x, y int) int {
	return int(p.Pix[y*p.W+x])
}

func (p *plane) prepareProfiles() {
	p.profX = xEdgeProfile(*p)
	p.profY = yEdgeProfile(*p)
}

func (p plane) xProfile() []float64 {
	if len(p.profX) == p.W {
		return p.profX
	}
	return xEdgeProfile(p)
}

func (p plane) yProfile() []float64 {
	if len(p.profY) == p.H {
		return p.profY
	}
	return yEdgeProfile(p)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func absFloat(v float64) float64 { return math.Abs(v) }

func median(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	cp := append([]float64(nil), samples...)
	sortFloats(cp)
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}

func sortFloats(v []float64) {
	for i := 1; i < len(v); i++ {
		x := v[i]
		j := i - 1
		for j >= 0 && v[j] > x {
			v[j+1] = v[j]
			j--
		}
		v[j+1] = x
	}
}

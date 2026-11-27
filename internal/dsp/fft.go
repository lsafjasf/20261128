package dsp

import (
	"fmt"
	"math"
	"math/cmplx"
)

// FFTPlan performs in-place radix-2 transforms of a fixed power-of-two size.
type FFTPlan struct {
	n     int
	rev   []int
	twice []complex128
}

// NewFFTPlan allocates a transform plan for n samples.
func NewFFTPlan(n int) (*FFTPlan, error) {
	if n < 2 || n&(n-1) != 0 {
		return nil, fmt.Errorf("FFT size %d is not a power of two >= 2", n)
	}
	rev := make([]int, n)
	bits := 0
	for (1 << bits) < n {
		bits++
	}
	for i := 0; i < n; i++ {
		rev[i] = reverseBits(i, bits)
	}
	twice := make([]complex128, n/2)
	for k := range twice {
		theta := -2 * math.Pi * float64(k) / float64(n)
		twice[k] = cmplx.Rect(1, theta)
	}
	return &FFTPlan{n: n, rev: rev, twice: twice}, nil
}

func reverseBits(v, bits int) int {
	var out int
	for i := 0; i < bits; i++ {
		out = (out << 1) | (v & 1)
		v >>= 1
	}
	return out
}

func (p *FFTPlan) Size() int { return p.n }

// Transform applies the forward transform. The inverse transform is scaled
// by 1/n so that IFFT(FFT(x)) == x.
func (p *FFTPlan) Transform(x []complex128, inverse bool) error {
	if len(x) != p.n {
		return fmt.Errorf("FFT input length %d, want %d", len(x), p.n)
	}
	for i, j := range p.rev {
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= p.n; size <<= 1 {
		step := p.n / size
		half := size >> 1
		for start := 0; start < p.n; start += size {
			for k := 0; k < half; k++ {
				w := p.twice[k*step]
				if inverse {
					w = cmplx.Conj(w)
				}
				a := x[start+k]
				b := x[start+k+half] * w
				x[start+k] = a + b
				x[start+k+half] = a - b
			}
		}
	}
	if inverse {
		scale := complex(1/float64(p.n), 0)
		for i := range x {
			x[i] *= scale
		}
	}
	return nil
}

// NextPowerOfTwo returns the smallest power of two >= n.
func NextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	out := 1
	for out < n {
		out <<= 1
	}
	return out
}

// Hann returns a periodic Hann window. For frame lengths above one this keeps
// the usual 0.5 overlap-add normalization.
func Hann(n int) []float64 {
	w := make([]float64, n)
	if n == 1 {
		w[0] = 1
		return w
	}
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n))
	}
	return w
}

// Hilbert returns the analytic signal x + j H(x) using a zero-padded FFT.
func Hilbert(x []float64) ([]complex128, error) {
	n := NextPowerOfTwo(len(x))
	if n < 2 {
		return nil, fmt.Errorf("Hilbert transform needs at least two samples")
	}
	plan, err := NewFFTPlan(n)
	if err != nil {
		return nil, err
	}
	buf := make([]complex128, n)
	for i, v := range x {
		buf[i] = complex(v, 0)
	}
	if err := plan.Transform(buf, false); err != nil {
		return nil, err
	}
	for k := 1; k < n/2; k++ {
		buf[k] *= 2
	}
	for k := n/2 + 1; k < n; k++ {
		buf[k] = 0
	}
	if err := plan.Transform(buf, true); err != nil {
		return nil, err
	}
	return buf[:len(x)], nil
}

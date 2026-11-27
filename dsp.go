package main

import "math"

// HighPass is a 2nd-order Butterworth high-pass biquad.
type HighPass struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func NewHighPass(cutoff, sampleRate float64) *HighPass {
	w := math.Tan(math.Pi * cutoff / sampleRate)
	n := 1 / (1 + math.Sqrt2*w + w*w)
	h := &HighPass{}
	h.b0 = n
	h.b1 = -2 * n
	h.b2 = n
	h.a1 = 2 * (w*w - 1) * n
	h.a2 = (1 - math.Sqrt2*w + w*w) * n
	return h
}

func (h *HighPass) Process(x float64) float64 {
	y := h.b0*x + h.b1*h.x1 + h.b2*h.x2 - h.a1*h.y1 - h.a2*h.y2
	h.x2, h.x1 = h.x1, x
	h.y2, h.y1 = h.y1, y
	return y
}

// LowPass is a 2nd-order Butterworth low-pass biquad.
type LowPass struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func NewLowPass(cutoff, sampleRate float64) *LowPass {
	w := math.Tan(math.Pi * cutoff / sampleRate)
	n := 1 / (1 + math.Sqrt2*w + w*w)
	l := &LowPass{}
	l.b0 = w * w * n
	l.b1 = 2 * l.b0
	l.b2 = l.b0
	l.a1 = 2 * (w*w - 1) * n
	l.a2 = (1 - math.Sqrt2*w + w*w) * n
	return l
}

func (l *LowPass) Process(x float64) float64 {
	y := l.b0*x + l.b1*l.x1 + l.b2*l.x2 - l.a1*l.y1 - l.a2*l.y2
	l.x2, l.x1 = l.x1, x
	l.y2, l.y1 = l.y1, y
	return y
}

// ZeroPhaseLowPass filters x forward and backward, canceling phase
// shift so program material does not leak into the LF estimate.
func ZeroPhaseLowPass(x []float64, cutoff, sampleRate float64) []float64 {
	fwd := NewLowPass(cutoff, sampleRate)
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = fwd.Process(v)
	}
	bwd := NewLowPass(cutoff, sampleRate)
	for i := len(y) - 1; i >= 0; i-- {
		y[i] = bwd.Process(y[i])
	}
	return y
}

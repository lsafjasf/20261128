package dsp

import (
	"fmt"
	"math"
)

// BoundaryMode controls the synthetic reference signal outside the available
// source samples.
type BoundaryMode int

const (
	BoundaryZero BoundaryMode = iota
	BoundaryReflect
	BoundaryClamp
)

func (m BoundaryMode) String() string {
	switch m {
	case BoundaryZero:
		return "zero"
	case BoundaryReflect:
		return "reflect"
	case BoundaryClamp:
		return "clamp"
	default:
		return "unknown"
	}
}

func ParseBoundary(s string) (BoundaryMode, error) {
	switch s {
	case "zero":
		return BoundaryZero, nil
	case "reflect":
		return BoundaryReflect, nil
	case "clamp":
		return BoundaryClamp, nil
	default:
		return 0, fmt.Errorf("unknown boundary mode %q", s)
	}
}

// SincResampler is a band-limited rational-ratio interpolator. Its reference
// impulse response is deliberately much longer than common production
// converters so that the latter's response remains visible in the residual.
type SincResampler struct {
	inRate   int
	outRate  int
	phases   int
	step     int
	half     int
	coeff    [][]float64
	relative []int
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

// NewSincResampler builds a Blackman-windowed sinc interpolator. cutoffHz is
// the reference passband edge and halfWidth is measured in source samples.
func NewSincResampler(inRate, outRate int, cutoffHz float64, halfWidth int) (*SincResampler, error) {
	if inRate <= 0 || outRate <= 0 {
		return nil, fmt.Errorf("sample rates must be positive")
	}
	if halfWidth < 4 {
		return nil, fmt.Errorf("sinc half-width must be at least 4 source samples")
	}
	nyquist := math.Min(float64(inRate), float64(outRate)) / 2
	if cutoffHz <= 0 || cutoffHz >= nyquist {
		return nil, fmt.Errorf("cutoff %.1f Hz must be between 0 and %.1f Hz", cutoffHz, nyquist)
	}
	g := gcd(inRate, outRate)
	phases := outRate / g
	relative := make([]int, 2*halfWidth)
	for i := range relative {
		relative[i] = i - halfWidth + 1
	}
	norm := 2 * cutoffHz / float64(inRate)
	coeff := make([][]float64, phases)
	for phase := 0; phase < phases; phase++ {
		frac := float64(phase) / float64(phases)
		row := make([]float64, len(relative))
		var sum float64
		for i, k := range relative {
			x := frac - float64(k)
			s := sinc(norm * x)
			t := x / float64(halfWidth)
			w := 0.0
			if math.Abs(t) <= 1 {
				w = 0.42 + 0.5*math.Cos(math.Pi*t) + 0.08*math.Cos(2*math.Pi*t)
			}
			row[i] = norm * s * w
			sum += row[i]
		}
		if math.Abs(sum) > 1e-15 {
			for i := range row {
				row[i] /= sum
			}
		}
		coeff[phase] = row
	}
	return &SincResampler{
		inRate: inRate, outRate: outRate, phases: phases,
		step: inRate / gcd(inRate, outRate),
		half: halfWidth, coeff: coeff, relative: relative,
	}, nil
}

func sinc(x float64) float64 {
	if math.Abs(x) < 1e-12 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

// OutputLength returns the number of output samples produced for n inputs.
func (r *SincResampler) OutputLength(n int) int {
	if n <= 0 {
		return 0
	}
	return int(math.Ceil(float64(n) * float64(r.outRate) / float64(r.inRate)))
}

// Resample evaluates the band-limited source on the destination sample grid.
func (r *SincResampler) Resample(in []float64, mode BoundaryMode) []float64 {
	return r.ResampleSegment(in, mode, 0, r.OutputLength(len(in)))
}

// ResampleSegment generates a contiguous destination range without
// materializing the rest of the file. Out-of-range source access still follows
// the requested boundary model.
func (r *SincResampler) ResampleSegment(in []float64, mode BoundaryMode, start, count int) []float64 {
	if len(in) == 0 {
		return nil
	}
	if start < 0 {
		start = 0
	}
	if count < 0 {
		count = 0
	}
	out := make([]float64, count)
	for i := 0; i < count; i++ {
		m := start + i
		phase := (m * r.step) % r.phases
		base := (m * r.step) / r.phases
		row := r.coeff[phase]
		var sum float64
		for i, rel := range r.relative {
			idx := base + rel
			v, ok := boundarySample(in, idx, mode)
			if ok {
				sum += v * row[i]
			}
		}
		out[i] = sum
	}
	return out
}

func boundarySample(x []float64, i int, mode BoundaryMode) (float64, bool) {
	n := len(x)
	if i >= 0 && i < n {
		return x[i], true
	}
	switch mode {
	case BoundaryZero:
		return 0, false
	case BoundaryReflect:
		if i < 0 {
			i = -i - 1
		} else if i >= n {
			i = 2*n - i - 1
		}
		i = int(Clamp(float64(i), 0, float64(n-1)))
		return x[i], true
	case BoundaryClamp:
		i = int(Clamp(float64(i), 0, float64(n-1)))
		return x[i], true
	default:
		return 0, false
	}
}

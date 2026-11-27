package dsp

import (
	"math"
	"testing"
)

func TestFFTRoundTrip(t *testing.T) {
	n := 1024
	plan, err := NewFFTPlan(n)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]complex128, n)
	for i := range want {
		want[i] = complex(math.Sin(float64(i)*0.13), math.Cos(float64(i)*0.07))
	}
	got := append([]complex128(nil), want...)
	if err := plan.Transform(got, false); err != nil {
		t.Fatal(err)
	}
	if err := plan.Transform(got, true); err != nil {
		t.Fatal(err)
	}
	for i := range want {
		if math.Abs(real(got[i])-real(want[i])) > 1e-10 || math.Abs(imag(got[i])-imag(want[i])) > 1e-10 {
			t.Fatalf("sample %d: got %v want %v", i, got[i], want[i])
		}
	}
}

func TestSincResamplerToneAndDelay(t *testing.T) {
	const inRate, outRate = 44100, 48000
	n := inRate
	in := make([]float64, n)
	for i := range in {
		in[i] = math.Sin(2 * math.Pi * 1000 * float64(i) / inRate)
	}
	rs, err := NewSincResampler(inRate, outRate, 20000, 64)
	if err != nil {
		t.Fatal(err)
	}
	out := rs.Resample(in, BoundaryReflect)
	if len(out) != 48000 {
		t.Fatalf("output length %d", len(out))
	}
	var errSum float64
	for i := 2000; i < 46000; i++ {
		want := math.Sin(2 * math.Pi * 1000 * float64(i) / outRate)
		d := out[i] - want
		errSum += d * d
	}
	rmse := math.Sqrt(errSum / 44000)
	if rmse > 2e-3 {
		t.Fatalf("1000 Hz tone RMSE %.6g", rmse)
	}
}

func TestBoundaryModesDifferAtEdge(t *testing.T) {
	in := []float64{0.2, 0.4, 0.6, 0.8, 1.0, 1.0, 1.0, 0.6, 0.2}
	rs, err := NewSincResampler(44100, 48000, 18000, 16)
	if err != nil {
		t.Fatal(err)
	}
	zero := rs.Resample(in, BoundaryZero)
	reflect := rs.Resample(in, BoundaryReflect)
	clamp := rs.Resample(in, BoundaryClamp)
	if math.Abs(zero[0]-reflect[0]) < 1e-6 {
		t.Fatalf("zero and reflect should differ, got %.9f", zero[0])
	}
	if math.Abs(reflect[len(reflect)-1]-clamp[len(clamp)-1]) < 1e-6 {
		t.Fatal("reflect and clamp should differ at the right edge")
	}
}

func TestUnwrapPhase(t *testing.T) {
	phase := []float64{3.0, -3.0, 3.0, -3.0}
	UnwrapPhase(phase)
	for i := 1; i < len(phase); i++ {
		if math.Abs(phase[i]-phase[i-1]) > math.Pi {
			t.Fatalf("unwrapped jump too large: %v", phase)
		}
	}
}

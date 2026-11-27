package stereo

import (
	"math"
	"testing"
)

const tol = 1e-9

func TestParallaxZeroAtConvergence(t *testing.T) {
	rig := SynthRig()
	if p := rig.ParallaxPx(1.5, rig.Convergence); math.Abs(p) > tol {
		t.Fatalf("parallax at screen plane = %v, want 0", p)
	}
}

func TestParallaxSignByDepth(t *testing.T) {
	rig := SynthRig()
	if p := rig.ParallaxPx(0, rig.Convergence/2); p >= 0 {
		t.Fatalf("in-front point should give crossed (negative) parallax, got %v", p)
	}
	if p := rig.ParallaxPx(0, rig.Convergence*2); p <= 0 {
		t.Fatalf("behind-screen point should give uncrossed (positive) parallax, got %v", p)
	}
}

func TestDepthForParallaxRoundTrip(t *testing.T) {
	rig := SynthRig()
	for _, z := range []float64{2, 5, 10, 30, 100} {
		p := rig.ParallaxPx(0.7, z)
		got := rig.DepthForParallax(p)
		if math.Abs(got-z) > 1e-6*z {
			t.Fatalf("round trip z=%v: got %v", z, got)
		}
	}
}

func TestDefaultBudgetCapsDivergence(t *testing.T) {
	rig := SynthRig()
	b := rig.DefaultBudget()
	ioPx := rig.Interaxial * rig.PixelsPerUnit
	if b.MaxUncrossed > ioPx+tol {
		t.Fatalf("uncrossed limit %v exceeds interocular %v px", b.MaxUncrossed, ioPx)
	}
	if b.MaxCrossed >= 0 || b.MaxUncrossed <= 0 {
		t.Fatalf("budget must straddle zero, got %+v", b)
	}
}

func TestClassify(t *testing.T) {
	b := Budget{MaxCrossed: -30, MaxUncrossed: 20}
	cases := []struct {
		p    float64
		want Level
	}{
		{0, LevelOK}, {-30, LevelOK}, {20, LevelOK},
		{-30.1, LevelStrain}, {20.1, LevelStrain},
	}
	for _, c := range cases {
		if got := b.Classify(c.p); got != c.want {
			t.Errorf("Classify(%v) = %v, want %v", c.p, got, c.want)
		}
	}
}

package stereo

import (
	"math"
	"testing"
)

func TestSynthPairsAreDeterministic(t *testing.T) {
	a, b := SynthPairs(42, 20), SynthPairs(42, 20)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("pair %d differs across runs: %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestSynthPairsContainBothLevels(t *testing.T) {
	budget := SynthRig().DefaultBudget()
	var ok, strain int
	for _, p := range SynthPairs(7, 100) {
		if p.Level(budget) == LevelOK {
			ok++
		} else {
			strain++
		}
	}
	if ok == 0 || strain == 0 {
		t.Fatalf("synthetic set should mix levels: ok=%d strain=%d", ok, strain)
	}
}

func TestMoveKeepsDepthInSync(t *testing.T) {
	for _, p := range SynthPairs(1, 50) {
		before := p.Disparity()
		p.MoveLeft(13.5)
		if got := p.Disparity(); math.Abs(got-before) > tol {
			t.Fatalf("MoveLeft changed disparity: %v -> %v", before, got)
		}
		p.MoveRight(-4.5)
		if math.Abs(p.Disparity()-before) > tol {
			t.Fatalf("MoveRight changed disparity")
		}
	}
}

func TestFixDisparityRestoresSafetyAndCenter(t *testing.T) {
	rig := SynthRig()
	budget := rig.DefaultBudget()
	fixed := 0
	for _, p := range SynthPairs(3, 200) {
		center := p.Center()
		if !p.FixDisparity(budget) {
			continue // already safe, untouched
		}
		fixed++
		if p.Level(budget) != LevelOK {
			t.Fatalf("still unsafe after fix: %+v", p)
		}
		if math.Abs(p.Center()-center) > tol {
			t.Fatalf("fix drifted center: %v -> %v", center, p.Center())
		}
	}
	if fixed == 0 {
		t.Fatal("synthetic set had nothing to fix")
	}
}

func TestNewPairAtDepthRoundTrip(t *testing.T) {
	rig := SynthRig()
	for _, z := range []float64{3, 10, 40} {
		p := NewPairAtDepth("x", 960, z, rig)
		if got := p.Depth(rig); math.Abs(got-z) > 1e-6*z {
			t.Fatalf("depth round trip z=%v: got %v", z, got)
		}
	}
}

package stereo

import (
	"fmt"
	"math/rand/v2"
)

// SynthRig is a fixed, cinema-like rig used by the synthetic data generator:
// 65 mm interaxial, 10 m convergence, 1920 px across a 6 m screen.
func SynthRig() Rig {
	return Rig{
		Interaxial:    0.065,
		Convergence:   10,
		PixelsPerUnit: 1920 / 6.0,
		ScreenWidthPx: 1920,
	}
}

// SynthPairs deterministically synthesizes n subtitle pairs for testing.
// Roughly every fifth pair is pushed outside the safe budget (alternating
// crossed/uncrossed) so both normal-stereo and strain-risk cases appear.
// The seed makes the data reproducible.
func SynthPairs(seed uint64, n int) []Pair {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b9))
	rig := SynthRig()
	budget := rig.DefaultBudget()
	pairs := make([]Pair, 0, n)
	for i := 0; i < n; i++ {
		center := 200 + rng.Float64()*(rig.ScreenWidthPx-400)
		var disparity float64
		if i%5 == 4 {
			// Unsafe: beyond the envelope on alternating sides.
			excess := 5 + rng.Float64()*30
			if i%10 == 4 {
				disparity = budget.MaxCrossed - excess
			} else {
				disparity = budget.MaxUncrossed + excess
			}
		} else {
			// Safe: uniform inside the envelope.
			span := budget.MaxUncrossed - budget.MaxCrossed
			disparity = budget.MaxCrossed + rng.Float64()*span
		}
		pairs = append(pairs, Pair{
			Text:   fmt.Sprintf("cue-%04d", i),
			LeftX:  center - disparity/2,
			RightX: center + disparity/2,
		})
	}
	return pairs
}

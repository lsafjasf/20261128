// stereocheck synthesizes stereoscopic subtitle cues, classifies each pair's
// parallax against the safe budget, and repairs unsafe pairs (updating both
// eyes together). Run: go run ./cmd/stereocheck [-n 20] [-seed 1]
package main

import (
	"flag"
	"fmt"

	"stereo-subtitle/stereo"
)

func main() {
	n := flag.Int("n", 20, "number of synthetic cues")
	seed := flag.Uint64("seed", 1, "synthesis seed")
	flag.Parse()

	rig := stereo.SynthRig()
	budget := rig.DefaultBudget()
	fmt.Printf("budget: crossed >= %.1f px, uncrossed <= %.1f px\n\n",
		budget.MaxCrossed, budget.MaxUncrossed)

	unsafe := 0
	for _, p := range stereo.SynthPairs(*seed, *n) {
		level := p.Level(budget)
		line := fmt.Sprintf("%-8s disp=%7.2f px depth=%7.2f m  %s",
			p.Text, p.Disparity(), p.Depth(rig), level)
		if level == stereo.LevelStrain {
			unsafe++
			p.FixDisparity(budget)
			line += fmt.Sprintf("  -> fixed: disp=%.2f px (both eyes synced)",
				p.Disparity())
		}
		fmt.Println(line)
	}
	fmt.Printf("\n%d/%d cues were strain risks and have been repaired.\n", unsafe, *n)
}

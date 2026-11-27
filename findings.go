package ghostdiag

import (
	"fmt"
	"sort"
)

func classifyComponents(
	original Plane,
	previous *Plane,
	residual []uint8,
	candidate []bool,
	temporalScores []float64,
	motionBox Box,
	vx, vy float64,
	opts Options,
) []Finding {
	w, h := original.Width, original.Height
	components := connectedComponents(candidate, w, h, opts.MinArea)
	findings := make([]Finding, 0, len(components))
	for _, component := range components {
		region := component.region()
		evidence := componentEvidence(
			original, previous, Plane{Width: w, Height: h, Pix: residual},
			temporalScores, component, motionBox, vx, vy, opts,
		)
		classification, secondary, score, reasons := classifyEvidence(evidence, opts)
		if classification == "" {
			continue
		}
		findings = append(findings, Finding{
			Classification: classification,
			Score:          score,
			Region:         region,
			Evidence:       evidence,
			Secondary:      secondary,
			Reasons:        reasons,
		})
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Score != findings[j].Score {
			return findings[i].Score > findings[j].Score
		}
		if findings[i].Region.Y != findings[j].Region.Y {
			return findings[i].Region.Y < findings[j].Region.Y
		}
		return findings[i].Region.X < findings[j].Region.X
	})
	if len(findings) > opts.MaxFindings {
		kept := append([]Finding(nil), findings[:opts.MaxFindings]...)
		hasGhost := false
		for _, finding := range kept {
			hasGhost = hasGhost || findingHasGhost(finding)
		}
		if !hasGhost {
			for _, finding := range findings[opts.MaxFindings:] {
				if findingHasGhost(finding) {
					kept[len(kept)-1] = finding
					break
				}
			}
		}
		findings = kept
	}
	return findings
}

func componentEvidence(
	original Plane,
	previous *Plane,
	residual Plane,
	temporalScores []float64,
	component component,
	motionBox Box,
	vx, vy float64,
	opts Options,
) Evidence {
	region := component.region()
	var temporalSum, residualSum, sourceSum float64
	for _, idx := range component.Pixels {
		temporalSum += temporalScores[idx]
		residualSum += float64(residual.Pix[idx])
		if previous != nil {
			sourceSum += float64(absInt(int(original.Pix[idx]) - int(previous.Pix[idx])))
		}
	}
	area := float64(len(component.Pixels))
	regionBox := Box{X: region.X, Y: region.Y, Width: region.Width, Height: region.Height}
	blockBox := expandBox(regionBox, 4, original.Width, original.Height)
	blockScore := blockiness(residual, blockBox, opts.BlockSize)
	if other := blockiness(residual, blockBox, 16); other > blockScore {
		blockScore = other
	}
	blurBox := expandBox(regionBox, 3, original.Width, original.Height)
	blurWidth := directionalBlurWidth(original, blurBox, vx, vy)
	overlapCount := 0
	for _, idx := range component.Pixels {
		x, y := idx%original.Width, idx/original.Width
		if insideBox(x, y, motionBox) {
			overlapCount++
		}
	}
	return Evidence{
		TemporalScore:  temporalSum / area,
		BlockScore:     blockScore,
		BlurWidth:      blurWidth,
		MotionOverlap:  float64(overlapCount) / area,
		SourceChange:   sourceSum / area,
		ResidualChange: residualSum / area,
	}
}

func classifyEvidence(evidence Evidence, opts Options) (string, []string, float64, []string) {
	ghost := evidence.TemporalScore >= opts.GhostThreshold
	block := evidence.BlockScore >= opts.BlockThreshold
	blur := evidence.BlurWidth >= opts.BlurWidthThreshold &&
		evidence.MotionOverlap >= 0.10
	var secondary []string
	var reasons []string
	switch {
	case ghost && evidence.TemporalScore >= evidence.BlockScore*0.85:
		if block {
			secondary = append(secondary, "coding_block")
			reasons = append(reasons, "denoised residual also clusters on a coding grid")
		}
		if blur {
			secondary = append(secondary, "motion_blur")
			reasons = append(reasons, "source edges are already widened along the motion vector")
		}
		reasons = append([]string{
			"denoised pixels match an older original frame better than the current one",
		}, reasons...)
		return "temporal_ghost", secondary, evidence.TemporalScore, reasons
	case block:
		if blur {
			secondary = append(secondary, "motion_blur")
		}
		reasons = append(reasons, "residual energy is concentrated on 8/16-pixel block borders")
		return "coding_block", secondary, evidence.BlockScore, reasons
	case blur:
		reasons = append(reasons,
			fmt.Sprintf("original edge transition is %.2f px wide along the motion direction",
				evidence.BlurWidth))
		return "motion_blur", nil, clamp01((evidence.BlurWidth - opts.BlurWidthThreshold) / opts.BlurWidthThreshold), reasons
	default:
		return "", nil, 0, nil
	}
}

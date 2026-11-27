package ghostdiag

import (
	"fmt"
	"math"
)

func (a *analyzer) push(orig, den Frame, explicitBox Box, hasExplicit bool) (FrameResult, error) {
	if err := orig.Plane.Validate(); err != nil {
		return FrameResult{}, fmt.Errorf("original frame %d: %w", orig.Index, err)
	}
	if err := den.Plane.Validate(); err != nil {
		return FrameResult{}, fmt.Errorf("denoised frame %d: %w", den.Index, err)
	}
	if orig.Plane.Width != den.Plane.Width || orig.Plane.Height != den.Plane.Height {
		return FrameResult{}, fmt.Errorf("frame %d geometry differs: original %dx%d, denoised %dx%d",
			orig.Index, orig.Plane.Width, orig.Plane.Height, den.Plane.Width, den.Plane.Height)
	}
	if a.width > 0 && (orig.Plane.Width != a.width || orig.Plane.Height != a.height) {
		return FrameResult{}, fmt.Errorf("frame %d geometry changed from %dx%d to %dx%d",
			orig.Index, a.width, a.height, orig.Plane.Width, orig.Plane.Height)
	}
	a.writeFingerprint(orig, den)
	w, h := orig.Plane.Width, orig.Plane.Height
	n := w * h
	changed := make([]bool, n)
	residual := make([]uint8, n)
	for i := 0; i < n; i++ {
		residual[i] = uint8(absInt(int(den.Plane.Pix[i]) - int(orig.Plane.Pix[i])))
		if a.prev != nil {
			if absInt(int(orig.Plane.Pix[i])-int(a.prev.Pix[i])) >= a.opts.MotionThreshold {
				changed[i] = true
			} else {
				for _, history := range a.history {
					if absInt(int(orig.Plane.Pix[i])-int(history.Pix[i])) >= a.opts.MotionThreshold {
						changed[i] = true
						break
					}
				}
			}
		}
	}
	box := Box{}
	hasBox := false
	if hasExplicit {
		box = clipBox(explicitBox, w, h)
		box.VX, box.VY = explicitBox.VX, explicitBox.VY
		hasBox = box.Width > 0 && box.Height > 0
		if hasBox {
			for y := box.Y; y < box.Y+box.Height; y++ {
				for x := box.X; x < box.X+box.Width; x++ {
					changed[y*w+x] = true
				}
			}
		}
	}
	if !hasBox {
		box, hasBox = bboxFromMask(changed, w, h)
	}
	if hasBox && a.prev != nil {
		if !hasExplicit || (explicitBox.VX == 0 && explicitBox.VY == 0) {
			box.VX, box.VY = estimateShift(*a.prev, orig.Plane, box, a.opts.MotionThreshold, 16)
		}
	} else if hasExplicit {
		box.VX, box.VY = explicitBox.VX, explicitBox.VY
	}

	candidate := make([]bool, n)
	motionPixels := 0
	for i := 0; i < n; i++ {
		if changed[i] {
			motionPixels++
		}
		if changed[i] && int(residual[i]) >= a.opts.NoiseThreshold {
			candidate[i] = true
		}
	}
	temporalScores := a.temporalScores(orig.Plane, den.Plane, candidate)
	result := FrameResult{
		Frame:        orig.Index,
		Timestamp:    orig.Timestamp,
		Trajectory:   box,
		MotionPixels: motionPixels,
	}
	if hasBox {
		result.SourceBlurWidth = directionalBlurWidth(orig.Plane, expandBox(box, 3, w, h), box.VX, box.VY)
		for _, size := range []int{8, 16} {
			score := blockiness(Plane{Width: w, Height: h, Pix: residual}, expandBox(box, 4, w, h), size)
			if score > result.CodingBlockScore {
				result.CodingBlockScore = score
			}
		}
	}
	result.BackgroundResidual, result.SourceNoise, result.NoiseReduction =
		a.backgroundMetrics(orig.Plane, den.Plane, changed)
	result.Findings = classifyComponents(
		orig.Plane, a.prev, residual, candidate, temporalScores,
		box, box.VX, box.VY, a.opts,
	)
	for i := range result.Findings {
		result.Findings[i].Frame = orig.Index
		result.Findings[i].Timestamp = orig.Timestamp
	}
	a.advanceHistory(orig.Plane)
	return result, nil
}

func (a *analyzer) temporalScores(orig, den Plane, candidate []bool) []float64 {
	scores := make([]float64, len(candidate))
	if a.prev == nil || len(a.history) == 0 {
		return scores
	}
	for i, use := range candidate {
		if !use {
			continue
		}
		r := int(den.Pix[i]) - int(orig.Pix[i])
		if absInt(r) < a.opts.NoiseThreshold {
			continue
		}
		best := 0.0
		for k := 0; k < len(a.history); k++ {
			history := a.history[len(a.history)-1-k]
			h := int(history.Pix[i]) - int(orig.Pix[i])
			if absInt(h) < a.opts.MotionThreshold {
				continue
			}
			alpha := float64(r*h) / float64(h*h)
			if alpha < 0.12 || alpha > 1.35 {
				continue
			}
			modelError := math.Abs(float64(r) - alpha*float64(h))
			correlation := 1 - modelError/(float64(absInt(r))+1)
			if correlation < 0.55 {
				continue
			}
			changeStrength := clamp01(math.Abs(float64(h)) / float64(2*a.opts.MotionThreshold))
			alphaStrength := clamp01(alpha * 1.25)
			score := correlation * math.Sqrt(changeStrength*alphaStrength)
			if score > best {
				best = score
			}
		}
		scores[i] = best
	}
	return scores
}

func (a *analyzer) backgroundMetrics(orig, den Plane, changed []bool) (float64, float64, float64) {
	if a.prev == nil {
		return 0, 0, 0
	}
	var residualSum, sourceSum int64
	var count int64
	for i := range changed {
		if changed[i] {
			continue
		}
		recentlyMoved := false
		for _, history := range a.history {
			if absInt(int(orig.Pix[i])-int(history.Pix[i])) >= a.opts.MotionThreshold {
				recentlyMoved = true
				break
			}
		}
		if recentlyMoved {
			continue
		}
		d := absInt(int(orig.Pix[i]) - int(a.prev.Pix[i]))
		residualSum += int64(absInt(int(den.Pix[i]) - int(orig.Pix[i])))
		sourceSum += int64(d)
		count++
	}
	if count == 0 {
		return 0, 0, 0
	}
	residualMAD := float64(residualSum) / float64(count)
	sourceMAD := float64(sourceSum) / float64(count)
	reduction := 0.0
	if sourceMAD > 0.01 {
		reduction = clamp01(1 - residualMAD/sourceMAD)
	}
	return residualMAD, sourceMAD, reduction
}

func (a *analyzer) advanceHistory(orig Plane) {
	clone := orig.Clone()
	a.history = append(a.history, clone)
	if len(a.history) > a.opts.HistoryFrames {
		copy(a.history, a.history[len(a.history)-a.opts.HistoryFrames:])
		a.history = a.history[:a.opts.HistoryFrames]
	}
	a.prev = &a.history[len(a.history)-1]
}

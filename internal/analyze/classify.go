package analyze

import (
	"fmt"
	"strings"
)

func classify(d frameData, order string) (string, float64, HypothesisScores, []string) {
	weave := clamp01(d.comb.Score)
	motion := motionBlurHypothesis(d.blur, d.effective)
	if d.smoothing.LineDuplicationRate >= 0.25 {
		motion *= 1 - 0.80*clamp01((d.smoothing.LineDuplicationRate-0.25)/0.40)
	}
	if motion >= 0.50 && d.smoothing.LineDuplicationRate < 0.20 {
		// A moving, genuinely blurred edge still produces parity differences.
		// Do not count that expected field-time difference as a weave failure.
		weave *= 0.30
	}
	if d.smoothing.LineDuplicationRate > 0.18 {
		weave *= 1 - 1.10*clamp01((d.smoothing.LineDuplicationRate-0.15)/0.50)
	}
	weave = clamp01(weave)
	smooth := smoothingHypothesis(d, weave)
	scores := HypothesisScores{Weave: weave, MotionBlur: motion, OverSmooth: smooth}

	verdict := VerdictInconclusive
	winner := 0.0
	switch {
	case weave >= 0.48 && d.comb.EdgeFraction >= 0.10:
		verdict, winner = VerdictWeaveError, weave
	case motion >= 0.50 && d.smoothing.LineDuplicationRate < 0.25 && motion >= smooth:
		verdict, winner = VerdictMotionBlur, motion
	case smooth >= 0.42:
		verdict, winner = VerdictOverSmooth, smooth
	case weave >= 0.38:
		verdict, winner = VerdictWeaveError, weave
	case d.blur.Direction == "sharp" && weave < 0.24 && smooth < 0.34:
		verdict, winner = VerdictClean, 0.55
	case motion >= 0.38 && motion >= smooth:
		verdict, winner = VerdictMotionBlur, motion
	}

	second := secondScore(verdict, scores)
	confidence := clamp01(0.48 + 0.34*winner + 0.18*maxFloat(0, winner-second))
	if verdict == VerdictClean || verdict == VerdictInconclusive {
		confidence = clamp01(0.50 + 0.30*(1-clamp01(maxFloat(weave, maxFloat(motion, smooth)))))
	}
	reasons := makeReasons(d, verdict, order)
	return verdict, confidence, scores, reasons
}

func motionBlurHypothesis(b BlurEvidence, motion Shift) float64 {
	if motion.Speed < 0.9 || motion.Confidence < 0.10 {
		return 0
	}
	alignment := motionBlurAlignment(b.Direction, motion)
	if alignment == 0 {
		return 0
	}
	width := 0.0
	switch b.Direction {
	case "horizontal":
		width = b.EdgeWidthH
	case "vertical":
		width = b.EdgeWidthV
	case "isotropic":
		width = mathMax(b.EdgeWidthH, b.EdgeWidthV)
	default:
		return 0
	}
	if width < 2.2 {
		return 0
	}
	widthScore := clamp01((width - 2.0) / 2.6)
	speedScore := clamp01((motion.Speed - 0.6) / 3.0)
	confScore := clamp01(motion.Confidence / 0.75)
	score := alignment * (0.55*widthScore + 0.30*speedScore + 0.15*confScore)
	if b.Direction == "isotropic" {
		score *= 0.82
	}
	return clamp01(score)
}

func motionBlurAlignment(blurDirection string, motion Shift) float64 {
	ax, ay := absFloat(motion.X), absFloat(motion.Y)
	if mathMax(ax, ay) < 0.8 {
		return 0
	}
	expected := "diagonal"
	switch {
	case ax >= 2*ay:
		expected = "horizontal"
	case ay >= 2*ax:
		expected = "vertical"
	}
	switch expected {
	case "horizontal":
		if blurDirection == "horizontal" {
			return 1
		}
		if blurDirection == "isotropic" {
			return 0.55
		}
	case "vertical":
		if blurDirection == "vertical" {
			return 1
		}
		if blurDirection == "isotropic" {
			return 0.55
		}
	default:
		if blurDirection == "isotropic" {
			return 0.9
		}
		if blurDirection == "horizontal" || blurDirection == "vertical" {
			return 0.55
		}
	}
	return 0
}

func smoothingScore(s SmoothingEvidence, b BlurEvidence, motion Shift) float64 {
	maxWidth := mathMax(b.EdgeWidthH, b.EdgeWidthV)
	lineScore := clamp01(s.LineDuplicationRate / 0.30)
	widthScore := clamp01((maxWidth - 2.0) / 2.8)
	detailScore := 0.0
	if s.LineDuplicationRate > 0.08 || widthScore > 0.30 {
		detailScore = clamp01((0.75 - s.VerticalDetailRatio) / 0.55)
	}
	temporalScore := 0.0
	if motion.Speed >= 0.8 {
		temporalScore = clamp01(s.TemporalDuplication / 0.55)
	}
	score := 0.55*lineScore + 0.25*widthScore + 0.10*detailScore + 0.10*temporalScore
	if b.Direction == "isotropic" {
		score += 0.05 * widthScore
	}
	if motionBlurAlignment(b.Direction, motion) >= 0.7 {
		score *= 0.55
	}
	return clamp01(score)
}

func smoothingHypothesis(d frameData, weave float64) float64 {
	score := smoothingScore(d.smoothing, d.blur, d.effective)
	if weave > 0.35 {
		score *= 1 - 0.85*clamp01((weave-0.35)/0.45)
	}
	return clamp01(score)
}

func secondScore(verdict string, s HypothesisScores) float64 {
	switch verdict {
	case VerdictWeaveError:
		return mathMax(s.MotionBlur, s.OverSmooth)
	case VerdictMotionBlur:
		return mathMax(s.Weave, s.OverSmooth)
	case VerdictOverSmooth:
		return mathMax(s.Weave, s.MotionBlur)
	default:
		return mathMax(s.Weave, mathMax(s.MotionBlur, s.OverSmooth))
	}
}

func makeReasons(d frameData, verdict, order string) []string {
	var reasons []string
	switch verdict {
	case VerdictWeaveError:
		reasons = append(reasons, "结论：运动边缘存在跨场奇偶行梳状残差，优先判为 weave 错误。")
	case VerdictMotionBlur:
		reasons = append(reasons, "结论：宽边方向与有符号场内/帧间运动一致，且行重复低；交叉场差异可由运动模糊与场间曝光解释。")
	case VerdictOverSmooth:
		reasons = append(reasons, "结论：梳状差异可由行复制解释，且行重复或方向细节下降明显，判为去隔行过度平滑。")
	case VerdictClean:
		reasons = append(reasons, "结论：未发现明显梳状残差、宽边模糊或行重复。")
	default:
		reasons = append(reasons, "结论：证据不足，不能可靠区分原始模糊与去隔行平滑。")
	}
	if order != "" {
		reasons = append(reasons, fmt.Sprintf("场序 %s；注意场序只解释时间顺序，不直接等同于 weave 质量。", strings.ToUpper(order)))
	}
	reasons = append(reasons, fmt.Sprintf(
		"梳状残差 %.2f：交叉场边缘占比 %.1f%%，归一化幅度 %.2f。",
		d.comb.Score, 100*d.comb.EdgeFraction, d.comb.Magnitude))
	reasons = append(reasons, fmt.Sprintf(
		"边缘过渡宽度 H=%.2f px / V=%.2f px（%s）；场内位移 %.1f px/场，帧间位移 %.1f px/帧，采用 %.1f px/场 %s，置信度 %.2f。",
		d.blur.EdgeWidthH, d.blur.EdgeWidthV, d.blur.Direction,
		d.intra.Speed, d.inter.Speed, d.effective.Speed, d.effective.Direction, d.effective.Confidence))
	reasons = append(reasons, fmt.Sprintf(
		"行重复率 %.1f%%，两场重复 %.1f%%，垂直细节比 %.2f。",
		100*d.smoothing.LineDuplicationRate, 100*d.smoothing.TemporalDuplication,
		d.smoothing.VerticalDetailRatio))
	return reasons
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 { return mathMax(a, b) }

package analyze

import "math"

func estimateShift(a, b plane, maxX, maxY int) Shift {
	profA, profB := a.xProfile(), b.xProfile()
	dx, scoreX, confX := bestLag(profA, profB, maxX)
	rowA, rowB := a.yProfile(), b.yProfile()
	dyRows, scoreY, confY := bestLag(rowA, rowB, maxY)

	x, y := 0.0, 0.0
	confidence := 0.0
	if profileVariance(profA)*profileVariance(profB) > 0 {
		x = float64(dx)
		confidence = math.Max(confidence, confX*clamp01(scoreX))
	}
	if profileVariance(rowA)*profileVariance(rowB) > 0 {
		y = float64(dyRows * 2)
		confidence = math.Max(confidence, confY*clamp01(scoreY))
	}
	speed := math.Hypot(x, y)
	return Shift{
		X:          x,
		Y:          y,
		Speed:      speed,
		Direction:  directionName(x, y),
		Confidence: confidence,
	}
}

func xEdgeProfile(p plane) []float64 {
	prof := make([]float64, p.W)
	for y := 0; y < p.H; y++ {
		for x := 1; x < p.W-1; x++ {
			prof[x] += math.Abs(float64(p.at(x+1, y) - p.at(x-1, y)))
		}
	}
	return prof
}

func yEdgeProfile(p plane) []float64 {
	prof := make([]float64, p.H)
	for y := 1; y < p.H-1; y++ {
		for x := 0; x < p.W; x++ {
			prof[y] += math.Abs(float64(p.at(x, y+1) - p.at(x, y-1)))
		}
	}
	return prof
}

func profileVariance(v []float64) float64 {
	if len(v) < 4 {
		return 0
	}
	var mean float64
	for _, x := range v {
		mean += x
	}
	mean /= float64(len(v))
	var variance float64
	for _, x := range v {
		d := x - mean
		variance += d * d
	}
	return variance / float64(len(v))
}

func bestLag(a, b []float64, maxLag int) (int, float64, float64) {
	if len(a) < 4 || len(b) < 4 || maxLag <= 0 {
		return 0, 0, 0
	}
	maxLag = minInt(maxLag, minInt(len(a), len(b))-1)
	minOverlap := maxInt(8, minInt(len(a), len(b))/2)
	bestLag, bestScore := 0, -2.0
	scores := make([]float64, 2*maxLag+1)
	for i := range scores {
		scores[i] = -2
	}
	for lag := -maxLag; lag <= maxLag; lag++ {
		start := maxInt(0, -lag)
		end := minInt(len(a), len(b)-lag)
		if end-start < minOverlap {
			continue
		}
		score := correlation(a[start:end], b[start+lag:end+lag])
		scores[lag+maxLag] = score
		if score > bestScore {
			bestLag, bestScore = lag, score
		}
	}
	if bestScore < 0 {
		return 0, 0, 0
	}
	secondScore := -2.0
	for lag := -maxLag; lag <= maxLag; lag++ {
		if absInt(lag-bestLag) <= 2 {
			continue
		}
		if scores[lag+maxLag] > secondScore {
			secondScore = scores[lag+maxLag]
		}
	}
	if secondScore < 0 {
		secondScore = 0
	}
	margin := (bestScore - secondScore) / (1 - secondScore + 1e-9)
	confidence := clamp01(0.25 + 0.75*clamp01(margin))
	if absInt(bestLag) == maxLag {
		confidence *= 0.65
	}
	if bestScore < 0.15 {
		confidence *= clamp01(bestScore / 0.15)
	}
	return bestLag, clamp(bestScore, 0, 1), clamp01(confidence)
}

func correlation(a, b []float64) float64 {
	if len(a) != len(b) || len(a) < 2 {
		return 0
	}
	var ma, mb float64
	for i := range a {
		ma += a[i]
		mb += b[i]
	}
	ma /= float64(len(a))
	mb /= float64(len(b))
	var ab, aa, bb float64
	for i := range a {
		da, db := a[i]-ma, b[i]-mb
		ab += da * db
		aa += da * da
		bb += db * db
	}
	if aa < 1e-9 || bb < 1e-9 {
		return 0
	}
	return ab / math.Sqrt(aa*bb)
}

func directionName(x, y float64) string {
	if math.Hypot(x, y) < 0.5 {
		return "static"
	}
	ax, ay := math.Abs(x), math.Abs(y)
	switch {
	case ax >= ay*2:
		if x > 0 {
			return "right"
		}
		return "left"
	case ay >= ax*2:
		if y > 0 {
			return "down"
		}
		return "up"
	default:
		switch {
		case x > 0 && y > 0:
			return "down-right"
		case x > 0:
			return "up-right"
		case y > 0:
			return "down-left"
		default:
			return "up-left"
		}
	}
}

func orderVectors(a, b, c, d plane, maxX, maxY int) []Shift {
	return []Shift{
		estimateShift(a, b, maxX, maxY),
		estimateShift(b, c, maxX, maxY),
		estimateShift(c, d, maxX, maxY),
	}
}

func scoreOrder(v []Shift) (float64, float64) {
	var sx, sy, sumMag, confSum float64
	var n int
	for _, s := range v {
		if s.Speed < 0.5 || s.Confidence < 0.10 {
			continue
		}
		sx += s.X
		sy += s.Y
		sumMag += s.Speed
		confSum += s.Confidence
		n++
	}
	if n < 2 || sumMag == 0 {
		return 0, 0
	}
	coherence := math.Hypot(sx, sy) / sumMag
	confidence := confSum / float64(n)
	return coherence * confidence, coherence
}

func chooseFieldOrder(prevTop, prevBottom, curTop, curBottom plane, maxX, maxY int) (string, float64) {
	tff := orderVectors(prevTop, prevBottom, curTop, curBottom, maxX, maxY)
	bff := []Shift{
		estimateShift(prevBottom, prevTop, maxX, maxY),
		estimateShift(prevTop, curBottom, maxX, maxY),
		estimateShift(curBottom, curTop, maxX, maxY),
	}
	tffScore, _ := scoreOrder(tff)
	bffScore, _ := scoreOrder(bff)
	if tffScore+bffScore < 0.12 {
		return "", 0
	}
	hi, lo := tffScore, bffScore
	order := "tff"
	if bffScore > tffScore {
		hi, lo = bffScore, tffScore
		order = "bff"
	}
	margin := (hi - lo) / (hi + 1e-9)
	return order, clamp01(0.55*hi + 0.45*margin)
}

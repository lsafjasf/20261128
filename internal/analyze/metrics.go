package analyze

import (
	"math"

	"interlace-diag/internal/video"
)

func measureComb(f video.Frame) CombEvidence {
	if f.Width < 3 || f.Height < 5 {
		return CombEvidence{}
	}
	var edgeN, combN int
	var sumGrad, sumComb, sumSame float64
	for y := 2; y < f.Height-2; y++ {
		row := y * f.Width
		for x := 1; x < f.Width-1; x++ {
			center := int(f.Y[row+x])
			gx := absInt(int(f.Y[row+x+1]) - int(f.Y[row+x-1]))
			if gx < 12 {
				continue
			}
			edgeN++
			cross := absInt(2*center - int(f.Y[row-f.Width+x]) - int(f.Y[row+f.Width+x]))
			same := (absInt(center-int(f.Y[row-2*f.Width+x])) +
				absInt(center-int(f.Y[row+2*f.Width+x]))) / 2
			sumGrad += float64(gx)
			sumComb += float64(cross)
			sumSame += float64(same)
			if cross >= 8 && cross*2 >= gx && cross*4 >= same*5 {
				combN++
			}
		}
	}
	if edgeN == 0 || sumGrad == 0 {
		return CombEvidence{}
	}
	e := CombEvidence{
		EdgeFraction:       float64(combN) / float64(edgeN),
		Magnitude:          clamp01((sumComb / sumGrad) / 2),
		SameParityResidual: clamp01((sumSame / sumGrad) / 2),
	}
	dominance := sumComb / (sumComb + sumSame + 1)
	e.Score = clamp01(0.55*(e.EdgeFraction/0.30) + 0.30*(e.Magnitude/0.55) + 0.15*dominance)
	return e
}

func measureLineDuplication(f video.Frame) (rate, verticalDetailRatio float64) {
	if f.Width < 2 || f.Height < 4 {
		return 0, 0
	}
	var valid, dup int
	var hfH, hfV float64
	var nH, nV int
	for y := 1; y < f.Height-1; y++ {
		var d1, d2 float64
		for x := 0; x < f.Width; x++ {
			d1 += math.Abs(float64(f.Y[y*f.Width+x]) - float64(f.Y[(y-1)*f.Width+x]))
			d2 += math.Abs(float64(f.Y[(y+1)*f.Width+x]) - float64(f.Y[y*f.Width+x]))
		}
		d1 /= float64(f.Width)
		d2 /= float64(f.Width)
		if d1+d2 >= 3 {
			valid++
			if d1 < 1.5 && d1 <= 0.22*d2 {
				dup++
			}
		}
		hfV += d1
		nV++
	}
	for y := 0; y < f.Height; y++ {
		row := y * f.Width
		for x := 1; x < f.Width; x++ {
			hfH += math.Abs(float64(f.Y[row+x]) - float64(f.Y[row+x-1]))
			nH++
		}
	}
	if valid > 0 {
		rate = float64(dup) / float64(valid)
	}
	if nH > 0 && nV > 0 {
		verticalDetailRatio = (hfV / float64(nV)) / (hfH/float64(nH) + 1)
	}
	return rate, verticalDetailRatio
}

func measureFields(top, bottom plane, intra Shift) (FieldEvidence, float64) {
	var raw, aligned float64
	var temporalDup, temporalN int
	n := 0
	for y := 0; y < top.H; y++ {
		for x := 0; x < top.W; x++ {
			a := top.at(x, y)
			b := bottom.at(x, y)
			raw += math.Abs(float64(a - b))
			n++
			gx := 0
			if x > 0 && x+1 < top.W {
				gx = absInt(top.at(x+1, y) - top.at(x-1, y))
			}
			if gx >= 12 {
				temporalN++
				if absInt(a-b) <= 2 {
					temporalDup++
				}
			}
		}
	}
	if n == 0 {
		return FieldEvidence{}, 0
	}
	raw /= float64(n)
	dx := int(math.Round(intra.X))
	dyRows := int(math.Round(intra.Y / 2))
	alignedN := 0
	for y := 0; y < top.H; y++ {
		yy := y + dyRows
		if yy < 0 || yy >= bottom.H {
			continue
		}
		for x := 0; x < top.W; x++ {
			xx := x + dx
			if xx < 0 || xx >= bottom.W {
				continue
			}
			aligned += math.Abs(float64(top.at(x, y) - bottom.at(xx, yy)))
			alignedN++
		}
	}
	if alignedN > 0 {
		aligned /= float64(alignedN)
	}
	residualRatio := 1.0
	if raw > 0.5 {
		residualRatio = clamp(aligned/raw, 0, 2)
	}
	dupRate := 0.0
	if temporalN > 0 {
		dupRate = float64(temporalDup) / float64(temporalN)
	}
	return FieldEvidence{
		TopBottomMAD:         raw,
		MotionCompensatedMAD: aligned,
		ResidualRatio:        residualRatio,
	}, dupRate
}

func planeMAD(a, b []byte) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += math.Abs(float64(a[i]) - float64(b[i]))
	}
	return sum / float64(len(a))
}

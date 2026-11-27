package analyze

import "math"

func measureBlur(fields []plane) BlurEvidence {
	var widthsH, widthsV []float64
	var detailH, detailV float64
	var nH, nV int
	for _, p := range fields {
		if p.W < 5 || p.H < 5 {
			continue
		}
		for y := 2; y < p.H-2; y++ {
			for x := 2; x < p.W-2; x++ {
				gx := math.Abs(float64(p.at(x+1, y)-p.at(x-1, y))) / 2
				gy := math.Abs(float64(p.at(x, y+1)-p.at(x, y-1))) / 2
				detailH += gx
				detailV += gy
				nH++
				nV++
				if gx >= 7 && isLocalMaxX(p, x, y) {
					if w, ok := edgeWidthH(p, x, y, gx); ok {
						widthsH = append(widthsH, w)
					}
				}
				if gy >= 7 && isLocalMaxY(p, x, y) {
					if w, ok := edgeWidthV(p, x, y, gy); ok {
						widthsV = append(widthsV, w)
					}
				}
			}
		}
	}
	e := BlurEvidence{}
	if nH > 0 {
		e.DetailH = detailH / float64(nH)
		e.DetailV = detailV / float64(nV)
	}
	if len(widthsH) >= 8 {
		e.EdgeWidthH = median(widthsH)
	}
	if len(widthsV) >= 8 {
		e.EdgeWidthV = median(widthsV)
	}
	e.Direction, e.DirectionalRatio = classifyBlur(e.EdgeWidthH, e.EdgeWidthV)
	return e
}

func isLocalMaxX(p plane, x, y int) bool {
	center := math.Abs(float64(p.at(x+1, y)-p.at(x-1, y))) / 2
	left := math.Abs(float64(p.at(x, y)-p.at(x-2, y))) / 2
	right := math.Abs(float64(p.at(x+2, y)-p.at(x, y))) / 2
	return center >= left*0.80 && center >= right*0.80
}

func isLocalMaxY(p plane, x, y int) bool {
	center := math.Abs(float64(p.at(x, y+1)-p.at(x, y-1))) / 2
	up := math.Abs(float64(p.at(x, y)-p.at(x, y-2))) / 2
	down := math.Abs(float64(p.at(x, y+2)-p.at(x, y))) / 2
	return center >= up*0.80 && center >= down*0.80
}

func edgeWidthH(p plane, x, y int, gradient float64) (float64, bool) {
	loX, hiX := maxInt(0, x-4), minInt(p.W-1, x+4)
	lo, hi := 255, 0
	for xx := loX; xx <= hiX; xx++ {
		v := p.at(xx, y)
		lo = minInt(lo, v)
		hi = maxInt(hi, v)
	}
	return widthFromRange(hi-lo, gradient)
}

func edgeWidthV(p plane, x, y int, gradient float64) (float64, bool) {
	loY, hiY := maxInt(0, y-4), minInt(p.H-1, y+4)
	lo, hi := 255, 0
	for yy := loY; yy <= hiY; yy++ {
		v := p.at(x, yy)
		lo = minInt(lo, v)
		hi = maxInt(hi, v)
	}
	return widthFromRange(hi-lo, gradient)
}

func widthFromRange(amplitude int, gradient float64) (float64, bool) {
	if amplitude < 24 || gradient <= 0 {
		return 0, false
	}
	w := float64(amplitude) / gradient
	if w < 1 || w > 20 || math.IsNaN(w) || math.IsInf(w, 0) {
		return 0, false
	}
	return w, true
}

func classifyBlur(h, v float64) (string, float64) {
	switch {
	case h == 0 && v == 0:
		return "unknown", 0
	case h == 0:
		if v >= 2.5 {
			return "vertical", 0
		}
		return "sharp", 0
	case v == 0:
		if h >= 2.5 {
			return "horizontal", 0
		}
		return "sharp", 0
	}
	ratio := math.Max(h, v) / math.Max(0.1, math.Min(h, v))
	if ratio >= 1.55 && math.Max(h, v) >= 2.5 {
		if h > v {
			return "horizontal", ratio
		}
		return "vertical", ratio
	}
	if h >= 2.8 && v >= 2.8 {
		return "isotropic", ratio
	}
	return "sharp", ratio
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

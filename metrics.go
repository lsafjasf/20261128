package ghostdiag

import (
	"math"
	"sort"
)

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampInt(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func clipBox(box Box, width, height int) Box {
	x0 := clampInt(box.X, 0, width)
	y0 := clampInt(box.Y, 0, height)
	x1 := clampInt(box.X+box.Width, 0, width)
	y1 := clampInt(box.Y+box.Height, 0, height)
	return Box{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}

func expandBox(box Box, amount, width, height int) Box {
	box.X -= amount
	box.Y -= amount
	box.Width += amount * 2
	box.Height += amount * 2
	return clipBox(box, width, height)
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

// blockiness compares residual gradients on an 8/16-pixel coding grid with
// gradients inside those blocks. A flat or gently varying residual confined to
// block borders scores high.
func blockiness(residual Plane, box Box, blockSize int) float64 {
	box = clipBox(box, residual.Width, residual.Height)
	if box.Width < 4 || box.Height < 4 {
		return 0
	}
	var boundarySum, boundaryN, interiorSum, interiorN float64
	for y := box.Y; y < box.Y+box.Height; y++ {
		for x := box.X; x < box.X+box.Width; x++ {
			idx := y*residual.Width + x
			var g float64
			if x > 0 {
				g = math.Abs(float64(residual.Pix[idx]) - float64(residual.Pix[idx-1]))
			}
			isBoundary := x > 0 && x%blockSize == 0
			if y > 0 {
				g += math.Abs(float64(residual.Pix[idx]) - float64(residual.Pix[idx-residual.Width]))
			}
			isBoundary = isBoundary || (y > 0 && y%blockSize == 0)
			if isBoundary {
				boundarySum += g
				boundaryN++
			} else {
				interiorSum += g
				interiorN++
			}
		}
	}
	if boundaryN == 0 || interiorN == 0 {
		return 0
	}
	boundary := boundarySum / boundaryN
	interior := interiorSum / interiorN
	if boundary < 2 {
		return 0
	}
	ratio := (boundary + 0.25) / (interior + 0.25)
	return clamp01((ratio - 1.25) / 1.75)
}

// directionalBlurWidth estimates the 10-90 transition width of strong source
// edges along the motion axis. A step edge is about one pixel wide; real motion
// blur widens it in the travel direction.
func directionalBlurWidth(source Plane, box Box, vx, vy float64) float64 {
	box = clipBox(box, source.Width, source.Height)
	if box.Width < 3 || box.Height < 3 {
		return 0
	}
	horizontal := math.Abs(vx) >= math.Abs(vy)
	var widths []float64
	if horizontal {
		for y := box.Y; y < box.Y+box.Height; y++ {
			widths = append(widths, scanLineWidths(source, box.X, box.X+box.Width, y, true)...)
		}
	} else {
		for x := box.X; x < box.X+box.Width; x++ {
			widths = append(widths, scanLineWidths(source, box.Y, box.Y+box.Height, x, false)...)
		}
	}
	return median(widths)
}

func scanLineWidths(p Plane, start, end, fixed int, horizontal bool) []float64 {
	if end-start < 3 {
		return nil
	}
	grad := make([]int, end-start)
	value := func(i int) int {
		if horizontal {
			return int(p.At(i, fixed))
		}
		return int(p.At(fixed, i))
	}
	for i := start + 1; i < end; i++ {
		grad[i-start] = value(i) - value(i-1)
	}
	var widths []float64
	for i := start + 1; i < end-1; i++ {
		pos := i - start
		g := absInt(grad[pos])
		if g < 15 || g < absInt(grad[pos-1]) || g < absInt(grad[pos+1]) {
			continue
		}
		sign := 1
		if grad[pos] < 0 {
			sign = -1
		}
		left := i
		for left > start+1 && absInt(grad[left-start])*5 >= g &&
			grad[left-start]*sign > 0 {
			left--
		}
		right := i + 1
		for right < end-1 && absInt(grad[right-start])*5 >= g &&
			grad[right-start]*sign > 0 {
			right++
		}
		contrast := value(right) - value(left)
		if sign < 0 {
			contrast = -contrast
		}
		if contrast < 24 {
			continue
		}
		width := float64(contrast) / float64(g)
		if width > 8 {
			width = 8
		}
		widths = append(widths, width)
	}
	return widths
}

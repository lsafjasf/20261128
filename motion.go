package ghostdiag

import "math"

func insideBox(x, y int, box Box) bool {
	return x >= box.X && y >= box.Y && x < box.X+box.Width && y < box.Y+box.Height
}

func bboxFromMask(mask []bool, width, height int) (Box, bool) {
	x0, y0, x1, y1 := width, height, -1, -1
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if !mask[y*width+x] {
				continue
			}
			if x < x0 {
				x0 = x
			}
			if y < y0 {
				y0 = y
			}
			if x+1 > x1 {
				x1 = x + 1
			}
			if y+1 > y1 {
				y1 = y + 1
			}
		}
	}
	if x1 <= x0 || y1 <= y0 {
		return Box{}, false
	}
	return Box{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}, true
}

// estimateShift finds the integer translation that best aligns moving source
// content between two frames. Only pixels that changed at zero shift vote, so
// the stationary background does not dominate.
func estimateShift(previous, current Plane, box Box, motionThreshold, radius int) (float64, float64) {
	box = clipBox(expandBox(box, radius, current.Width, current.Height), current.Width, current.Height)
	if box.Width <= 0 || box.Height <= 0 {
		return 0, 0
	}
	step := 1
	area := box.Width * box.Height
	if area > 6000 {
		step = int(math.Ceil(math.Sqrt(float64(area) / 6000)))
	}
	type point struct{ x, y int }
	var points []point
	for y := box.Y; y < box.Y+box.Height; y += step {
		for x := box.X; x < box.X+box.Width; x += step {
			idx := y*current.Width + x
			if absInt(int(current.Pix[idx])-int(previous.Pix[idx])) >= motionThreshold {
				points = append(points, point{x, y})
			}
		}
	}
	if len(points) < 8 {
		return 0, 0
	}
	bestX, bestY, bestScore := 0, 0, int64(1<<62)
	zeroScore := int64(0)
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			var score int64
			valid := 0
			for _, p := range points {
				sx, sy := p.x-dx, p.y-dy
				if sx < 0 || sx >= current.Width || sy < 0 || sy >= current.Height {
					continue
				}
				score += int64(absInt(int(current.At(p.x, p.y)) - int(previous.At(sx, sy))))
				valid++
			}
			if valid < len(points)*3/4 {
				continue
			}
			score = score * int64(len(points)) / int64(valid)
			if dx == 0 && dy == 0 {
				zeroScore = score
			}
			if score < bestScore {
				bestScore, bestX, bestY = score, dx, dy
			}
		}
	}
	if bestScore >= zeroScore {
		return 0, 0
	}
	return float64(bestX), float64(bestY)
}

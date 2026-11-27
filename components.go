package ghostdiag

type component struct {
	X0, Y0 int
	X1, Y1 int
	Pixels []int
}

func (c component) region() Region {
	return Region{
		X:      c.X0,
		Y:      c.Y0,
		Width:  c.X1 - c.X0,
		Height: c.Y1 - c.Y0,
		Area:   len(c.Pixels),
	}
}

func connectedComponents(mask []bool, width, height, minArea int) []component {
	seen := make([]bool, len(mask))
	queue := make([]int, 0, 256)
	var result []component
	for start, set := range mask {
		if !set || seen[start] {
			continue
		}
		queue = queue[:0]
		queue = append(queue, start)
		seen[start] = true
		c := component{X0: width, Y0: height}
		for head := 0; head < len(queue); head++ {
			idx := queue[head]
			x, y := idx%width, idx/width
			if x < c.X0 {
				c.X0 = x
			}
			if y < c.Y0 {
				c.Y0 = y
			}
			if x+1 > c.X1 {
				c.X1 = x + 1
			}
			if y+1 > c.Y1 {
				c.Y1 = y + 1
			}
			c.Pixels = append(c.Pixels, idx)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || nx >= width || ny < 0 || ny >= height {
						continue
					}
					next := ny*width + nx
					if mask[next] && !seen[next] {
						seen[next] = true
						queue = append(queue, next)
					}
				}
			}
		}
		if len(c.Pixels) >= minArea {
			result = append(result, c)
		}
	}
	return result
}

func dilate3x3(mask []bool, width, height int) []bool {
	out := make([]bool, len(mask))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if !mask[y*width+x] {
				continue
			}
			for dy := -1; dy <= 1; dy++ {
				ny := y + dy
				if ny < 0 || ny >= height {
					continue
				}
				for dx := -1; dx <= 1; dx++ {
					nx := x + dx
					if nx >= 0 && nx < width {
						out[ny*width+nx] = true
					}
				}
			}
		}
	}
	return out
}

func erode3x3(mask []bool, width, height int) []bool {
	out := make([]bool, len(mask))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if !mask[y*width+x] {
				continue
			}
			keep := true
			for dy := -1; dy <= 1 && keep; dy++ {
				ny := y + dy
				if ny < 0 || ny >= height {
					keep = false
					break
				}
				for dx := -1; dx <= 1; dx++ {
					nx := x + dx
					if nx < 0 || nx >= width || !mask[ny*width+nx] {
						keep = false
						break
					}
				}
			}
			out[y*width+x] = keep
		}
	}
	return out
}

func opening3x3(mask []bool, width, height int) []bool {
	return dilate3x3(erode3x3(mask, width, height), width, height)
}

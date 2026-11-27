// Package synth builds deterministic paired frames for tests and examples.
package synth

import (
	"bytes"
	"fmt"
	"io"
)

// Point is one explicit motion-trajectory sample.
type Point struct {
	Frame  int     `json:"frame"`
	X      int     `json:"x"`
	Y      int     `json:"y"`
	Width  int     `json:"w"`
	Height int     `json:"h"`
	VX     float64 `json:"vx"`
	VY     float64 `json:"vy"`
}

// Sequence is a reproducible pair of original and denoised luma sequences.
type Sequence struct {
	Width      int
	Height     int
	FrameRate  float64
	Original   [][]byte
	Denoised   [][]byte
	Trajectory []Point
	FirstGhost int
}

// Build creates a moving actor with real source motion blur. The denoiser
// removes background noise, but from frame 2 onward leaves faded copies of
// previous actor positions. Frame 1 also carries an 8x8 block-coded patch.
func Build() Sequence {
	const (
		width      = 160
		height     = 96
		frames     = 9
		actorX0    = 20
		actorStep  = 12
		actorY     = 25
		actorW     = 18
		actorH     = 46
		firstGhost = 2
	)
	seq := Sequence{
		Width: width, Height: height, FrameRate: 25,
		Original: make([][]byte, frames), Denoised: make([][]byte, frames),
		FirstGhost: firstGhost,
	}
	for frame := 0; frame < frames; frame++ {
		ax := actorX0 + frame*actorStep
		original := make([]byte, width*height)
		denoised := make([]byte, width*height)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				idx := y*width + x
				background := cleanBackground(x, y, width, height)
				original[idx] = clampByte(background + sensorNoise(x, y, frame))
				denoised[idx] = clampByte(background)
			}
		}
		if frame == 1 {
			applyBlockPatch(denoised, width, height, 16, actorY-1, 32, actorH+2)
		}
		for trail := 1; trail <= 3 && frame >= firstGhost && frame-trail >= 0; trail++ {
			alpha := 0.82 - 0.22*float64(trail-1)
			drawActor(denoised, width, height, ax-trail*actorStep, actorY, actorW, actorH, alpha)
		}
		drawActor(original, width, height, ax, actorY, actorW, actorH, 1)
		drawActor(denoised, width, height, ax, actorY, actorW, actorH, 1)
		seq.Original[frame] = original
		seq.Denoised[frame] = denoised
		seq.Trajectory = append(seq.Trajectory, Point{
			Frame: frame, X: ax - 4, Y: actorY, Width: actorW + 8, Height: actorH,
			VX: actorStep, VY: 0,
		})
	}
	return seq
}

func cleanBackground(x, y, width, height int) int {
	return 52 + (x*35)/width + (y*18)/height + ((x/9+y/7)%3)*4
}

func sensorNoise(x, y, frame int) int {
	v := uint32(x*374761393 + y*668265263 + frame*2246822519)
	v ^= v >> 13
	v *= 1274126177
	v ^= v >> 16
	return int(v%11) - 5
}

func drawActor(dst []byte, width, height, ax, ay, actorW, actorH int, alpha float64) {
	for y := ay; y < ay+actorH && y < height; y++ {
		for x := ax - 4; x < ax+actorW+4 && x < width; x++ {
			if x < 0 {
				continue
			}
			coverage := actorCoverage(x, ax, actorW)
			if coverage <= 0 {
				continue
			}
			idx := y*width + x
			value := (1-alpha*coverage)*float64(dst[idx]) + alpha*coverage*196
			dst[idx] = clampByte(int(value + 0.5))
		}
	}
}

func actorCoverage(x, ax, actorW int) float64 {
	if x < ax-4 || x >= ax+actorW+4 {
		return 0
	}
	left := float64(x-(ax-4)) / 4
	right := float64(ax+actorW+4-x) / 4
	coverage := left
	if right < coverage {
		coverage = right
	}
	if coverage > 1 {
		return 1
	}
	if coverage < 0 {
		return 0
	}
	return coverage
}

func applyBlockPatch(dst []byte, width, height, x0, y0, w, h int) {
	for y := y0; y < y0+h && y < height; y++ {
		for x := x0; x < x0+w && x < width; x++ {
			if x < 0 || y < 0 {
				continue
			}
			offset := 9
			if ((x/8)+(y/8))%2 == 0 {
				offset = 24
			}
			dst[y*width+x] = clampByte(int(dst[y*width+x]) + offset)
		}
	}
}

func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// WriteY4M writes one 8-bit 4:2:0 sequence.
func WriteY4M(w io.Writer, seq Sequence, frames [][]byte) error {
	if _, err := fmt.Fprintf(w, "YUV4MPEG2 W%d H%d F25:1 Ip C420jpeg\n", seq.Width, seq.Height); err != nil {
		return err
	}
	var chroma bytes.Buffer
	chroma.Write(bytes.Repeat([]byte{128}, 2*((seq.Width+1)/2)*((seq.Height+1)/2)))
	for _, frame := range frames {
		if _, err := io.WriteString(w, "FRAME\n"); err != nil {
			return err
		}
		if _, err := w.Write(frame); err != nil {
			return err
		}
		if _, err := w.Write(chroma.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

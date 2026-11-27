package yuv

import (
	"fmt"
	"math"
)

type ConvertOptions struct {
	Matrix      Matrix         `json:"matrix"`
	Range       Range          `json:"range"`
	Chroma      ChromaLocation `json:"chroma_location"`
	RGBBitDepth int            `json:"rgb_bit_depth"`
}

func DefaultConvertOptions() ConvertOptions {
	return ConvertOptions{
		Matrix:      MatrixBT709,
		Range:       Limited,
		Chroma:      ChromaLeft,
		RGBBitDepth: 8,
	}
}

type RGBPixel struct {
	R       uint16 `json:"r"`
	G       uint16 `json:"g"`
	B       uint16 `json:"b"`
	Clipped bool   `json:"clipped"`
}

type RGBFrame struct {
	Width    int
	Height   int
	BitDepth int
	Pix      []RGBPixel
}

type RGBViolation struct {
	X       int     `json:"x"`
	Y       int     `json:"y"`
	R       float64 `json:"r_unclipped"`
	G       float64 `json:"g_unclipped"`
	B       float64 `json:"b_unclipped"`
	Clipped string  `json:"channels_clipped"`
}

type ConversionAudit struct {
	Pixels             int            `json:"pixels"`
	OutOfGamutPixels   int            `json:"out_of_gamut_pixels"`
	OutR               int            `json:"out_r"`
	OutG               int            `json:"out_g"`
	OutB               int            `json:"out_b"`
	ClippedSamples     int            `json:"clipped_samples"`
	UpsampleOvershoots int            `json:"upsample_overshoots"`
	Violations         []RGBViolation `json:"violations"`
	Truncated          bool           `json:"violations_truncated"`
}

func ConvertFrame(f *Frame, opts ConvertOptions, maxViolations int) (*RGBFrame, ConversionAudit, error) {
	var audit ConversionAudit
	if err := f.Validate(); err != nil {
		return nil, audit, err
	}
	if opts.RGBBitDepth < 8 || opts.RGBBitDepth > 16 {
		return nil, audit, fmt.Errorf("unsupported RGB bit depth %d", opts.RGBBitDepth)
	}
	if maxViolations < 0 {
		return nil, audit, fmt.Errorf("negative violation limit")
	}
	kr, kb, err := opts.Matrix.krkb()
	if err != nil {
		return nil, audit, err
	}
	params, err := normalization(f.BitDepth, opts.Range)
	if err != nil {
		return nil, audit, err
	}
	cbUp, err := upsamplePlane(f.Cb, f.Width, f.Height, opts.Chroma)
	if err != nil {
		return nil, audit, err
	}
	crUp, err := upsamplePlane(f.Cr, f.Width, f.Height, opts.Chroma)
	if err != nil {
		return nil, audit, err
	}
	cbMin, cbMax := planeRange(f.Cb)
	crMin, crMax := planeRange(f.Cr)
	kg := 1 - kr - kb
	crToR := 2 * (1 - kr)
	cbToB := 2 * (1 - kb)
	cbToG := 2 * kb * (1 - kb) / kg
	crToG := 2 * kr * (1 - kr) / kg
	out := &RGBFrame{
		Width: f.Width, Height: f.Height, BitDepth: opts.RGBBitDepth,
		Pix: make([]RGBPixel, f.Width*f.Height),
	}
	maxRGB := (1 << opts.RGBBitDepth) - 1
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			i := y*f.Width + x
			if int(cbUp[i]) < cbMin || int(cbUp[i]) > cbMax ||
				int(crUp[i]) < crMin || int(crUp[i]) > crMax {
				audit.UpsampleOvershoots++
			}
			yn := float64(int(f.Y.Sample(x, y))-params.yOffset) * params.yScale
			cbn := float64(int(cbUp[i])-params.cOffset) * params.cScale
			crn := float64(int(crUp[i])-params.cOffset) * params.cScale
			rf := yn + crToR*crn
			gf := yn - cbToG*cbn - crToG*crn
			bf := yn + cbToB*cbn
			scale := float64(maxRGB)
			r, ro := quantizeCode(rf*scale, maxRGB)
			g, gout := quantizeCode(gf*scale, maxRGB)
			b, bo := quantizeCode(bf*scale, maxRGB)
			clipped := ro || gout || bo
			out.Pix[i] = RGBPixel{R: uint16(r), G: uint16(g), B: uint16(b), Clipped: clipped}
			audit.Pixels++
			if ro {
				audit.OutR++
			}
			if gout {
				audit.OutG++
			}
			if bo {
				audit.OutB++
			}
			if clipped {
				audit.OutOfGamutPixels++
				audit.ClippedSamples += btoi(ro) + btoi(gout) + btoi(bo)
				if len(audit.Violations) < maxViolations {
					audit.Violations = append(audit.Violations, RGBViolation{
						X: x, Y: y, R: round6(rf * scale), G: round6(gf * scale), B: round6(bf * scale),
						Clipped: clipNames(ro, gout, bo),
					})
				} else {
					audit.Truncated = true
				}
			}
		}
	}
	return out, audit, nil
}

type normalizer struct {
	yOffset int
	yScale  float64
	cOffset int
	cScale  float64
}

func normalization(bitDepth int, rng Range) (normalizer, error) {
	yLo, yHi, cLo, cHi, err := rng.Bounds(bitDepth)
	if err != nil {
		return normalizer{}, err
	}
	n := normalizer{cOffset: 1 << (bitDepth - 1)}
	if rng == Limited {
		n.yOffset = yLo
		n.yScale = 1 / float64(yHi-yLo)
		n.cScale = 1 / float64(cHi-cLo)
		return n, nil
	}
	max := (1 << bitDepth) - 1
	n.yScale = 1 / float64(max)
	n.cScale = 1 / float64(max)
	return n, nil
}

func planeRange(p Plane) (int, int) {
	lo, hi := int(^uint(0)>>1), 0
	for y := 0; y < p.Height; y++ {
		for x := 0; x < p.Width; x++ {
			v := int(p.Sample(x, y))
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
	}
	return lo, hi
}

func btoi(v bool) int {
	if v {
		return 1
	}
	return 0
}

func clipNames(r, g, b bool) string {
	var s string
	if r {
		s += "R"
	}
	if g {
		s += "G"
	}
	if b {
		s += "B"
	}
	return s
}

func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

package yuv

import "fmt"

// Violation is an exact, pre-clipping code-value violation.
type Violation struct {
	Plane  string `json:"plane"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Value  int    `json:"value"`
	Low    int    `json:"low"`
	High   int    `json:"high"`
	Delta  int    `json:"delta"`
	Origin string `json:"origin,omitempty"`
}

type PlaneAudit struct {
	Width      int `json:"width"`
	Height     int `json:"height"`
	Samples    int `json:"samples"`
	Min        int `json:"min"`
	Max        int `json:"max"`
	Below      int `json:"below"`
	Above      int `json:"above"`
	OutOfRange int `json:"out_of_range"`
}

type SampleAudit struct {
	Range               Range       `json:"range"`
	BitDepth            int         `json:"bit_depth"`
	Y                   PlaneAudit  `json:"y"`
	Cb                  PlaneAudit  `json:"cb"`
	Cr                  PlaneAudit  `json:"cr"`
	OutOfRangeSamples   int         `json:"out_of_range_samples"`
	AffectedPixels      int         `json:"affected_luma_pixels"`
	BitDepthAnomalies   int         `json:"bit_depth_anomalies"`
	PaddingAnomalies    int         `json:"padding_anomalies"`
	Violations          []Violation `json:"violations"`
	ViolationsTruncated bool        `json:"violations_truncated"`
}

func InspectSamples(f *Frame, rng Range, maxViolations int) (SampleAudit, error) {
	if err := f.Validate(); err != nil {
		return SampleAudit{}, err
	}
	yLo, yHi, cLo, cHi, err := rng.Bounds(f.BitDepth)
	if err != nil {
		return SampleAudit{}, err
	}
	if maxViolations < 0 {
		return SampleAudit{}, fmt.Errorf("negative violation limit")
	}
	a := SampleAudit{
		Range:             rng,
		BitDepth:          f.BitDepth,
		BitDepthAnomalies: f.BitDepthAnomaly,
		PaddingAnomalies:  f.PaddingAnomaly,
	}
	affected := make([]bool, f.Width*f.Height)
	markAffected := func(v Violation) {
		if v.Plane == "Y" {
			affected[v.Y*f.Width+v.X] = true
			return
		}
		for yy := v.Y * 2; yy < v.Y*2+2 && yy < f.Height; yy++ {
			for xx := v.X * 2; xx < v.X*2+2 && xx < f.Width; xx++ {
				affected[yy*f.Width+xx] = true
			}
		}
	}
	add := func(v Violation) {
		markAffected(v)
		if len(a.Violations) < maxViolations {
			a.Violations = append(a.Violations, v)
		} else {
			a.ViolationsTruncated = true
		}
	}

	a.Y = inspectPlane(f.Y, yLo, yHi, add)
	a.Cb = inspectPlane(f.Cb, cLo, cHi, add)
	a.Cr = inspectPlane(f.Cr, cLo, cHi, add)
	a.OutOfRangeSamples = a.Y.OutOfRange + a.Cb.OutOfRange + a.Cr.OutOfRange
	for _, v := range affected {
		if v {
			a.AffectedPixels++
		}
	}
	return a, nil
}

// SourceViolationMask marks luma-grid pixels whose own luma or covering 2x2
// chroma sample is outside the selected legal range.
func SourceViolationMask(f *Frame, rng Range) ([]bool, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	yLo, yHi, cLo, cHi, err := rng.Bounds(f.BitDepth)
	if err != nil {
		return nil, err
	}
	mask := make([]bool, f.Width*f.Height)
	check := func(p Plane, lo, hi int) {
		for y := 0; y < p.Height; y++ {
			for x := 0; x < p.Width; x++ {
				v := int(p.Sample(x, y))
				if v >= lo && v <= hi {
					continue
				}
				if p.Name == "Y" {
					mask[y*f.Width+x] = true
					continue
				}
				for yy := y * 2; yy < y*2+2 && yy < f.Height; yy++ {
					for xx := x * 2; xx < x*2+2 && xx < f.Width; xx++ {
						mask[yy*f.Width+xx] = true
					}
				}
			}
		}
	}
	check(f.Y, yLo, yHi)
	check(f.Cb, cLo, cHi)
	check(f.Cr, cLo, cHi)
	return mask, nil
}

func inspectPlane(p Plane, lo, hi int, add func(Violation)) PlaneAudit {
	a := PlaneAudit{
		Width:   p.Width,
		Height:  p.Height,
		Samples: p.Width * p.Height,
		Min:     int(^uint(0) >> 1),
	}
	for y := 0; y < p.Height; y++ {
		for x := 0; x < p.Width; x++ {
			v := int(p.Sample(x, y))
			if v < a.Min {
				a.Min = v
			}
			if v > a.Max {
				a.Max = v
			}
			o := Violation{Plane: p.Name, X: x, Y: y, Value: v, Low: lo, High: hi}
			switch {
			case v < lo:
				a.Below++
				a.OutOfRange++
				o.Delta = v - lo
				add(o)
			case v > hi:
				a.Above++
				a.OutOfRange++
				o.Delta = v - hi
				add(o)
			}
		}
	}
	if a.Samples == 0 {
		a.Min = 0
	}
	return a
}

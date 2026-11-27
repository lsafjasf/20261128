package yuv

import "fmt"

type Origin string

const (
	OriginInput         Origin = "input"
	OriginDecode        Origin = "decode"
	OriginInputOrDecode Origin = "input_or_decode"
	OriginConversion    Origin = "conversion"
)

type Classification struct {
	ReferenceProvided        bool        `json:"reference_provided"`
	DecodedOutOfRangeSamples int         `json:"decoded_out_of_range_samples"`
	InputSamples             int         `json:"input_samples"`
	DecodeSamples            int         `json:"decode_samples"`
	InputOrDecodeSamples     int         `json:"input_or_decode_samples"`
	InputPixels              int         `json:"input_pixels"`
	DecodePixels             int         `json:"decode_pixels"`
	InputOrDecodePixels      int         `json:"input_or_decode_pixels"`
	TrueOutOfRangePixels     int         `json:"true_out_of_range_pixels"`
	FormatAnomalies          int         `json:"format_anomalies"`
	Violations               []Violation `json:"violations"`
	ViolationsTruncated      bool        `json:"violations_truncated"`
}

// ClassifySamples attributes decoded code-value violations. A reference frame
// is optional. Without one, legal-range overflow inside the declared bit depth
// cannot be attributed between upstream input and the decoder.
func ClassifySamples(decoded, reference *Frame, rng Range, maxViolations int) (Classification, error) {
	if err := decoded.Validate(); err != nil {
		return Classification{}, err
	}
	if maxViolations < 0 {
		return Classification{}, fmt.Errorf("negative violation limit")
	}
	if reference != nil {
		if err := reference.Validate(); err != nil {
			return Classification{}, fmt.Errorf("invalid reference frame: %w", err)
		}
		if reference.Width != decoded.Width || reference.Height != decoded.Height ||
			reference.BitDepth != decoded.BitDepth {
			return Classification{}, fmt.Errorf("reference frame geometry/bit depth does not match decoded frame")
		}
	}
	yLo, yHi, cLo, cHi, err := rng.Bounds(decoded.BitDepth)
	if err != nil {
		return Classification{}, err
	}
	maxCode := (1 << decoded.BitDepth) - 1
	result := Classification{ReferenceProvided: reference != nil}
	masks := make([]uint8, decoded.Width*decoded.Height)
	any := make([]bool, len(masks))
	const (
		bitInput uint8 = 1 << iota
		bitDecode
		bitUnknown
	)
	mark := func(plane string, x, y int, bit uint8) {
		if plane == "Y" {
			masks[y*decoded.Width+x] |= bit
			any[y*decoded.Width+x] = true
			return
		}
		for yy := y * 2; yy < y*2+2 && yy < decoded.Height; yy++ {
			for xx := x * 2; xx < x*2+2 && xx < decoded.Width; xx++ {
				idx := yy*decoded.Width + xx
				masks[idx] |= bit
				any[idx] = true
			}
		}
	}
	planes := []struct {
		name string
		dec  Plane
		ref  Plane
		lo   int
		hi   int
	}{
		{"Y", decoded.Y, Plane{}, yLo, yHi},
		{"Cb", decoded.Cb, Plane{}, cLo, cHi},
		{"Cr", decoded.Cr, Plane{}, cLo, cHi},
	}
	if reference != nil {
		planes[0].ref = reference.Y
		planes[1].ref = reference.Cb
		planes[2].ref = reference.Cr
	}
	for _, pl := range planes {
		for y := 0; y < pl.dec.Height; y++ {
			for x := 0; x < pl.dec.Width; x++ {
				value := int(pl.dec.Sample(x, y))
				if value >= pl.lo && value <= pl.hi {
					continue
				}
				result.DecodedOutOfRangeSamples++
				origin := OriginInputOrDecode
				bit := bitUnknown
				if reference != nil {
					rv := int(pl.ref.Sample(x, y))
					if rv < pl.lo || rv > pl.hi {
						origin, bit = OriginInput, bitInput
					} else {
						origin, bit = OriginDecode, bitDecode
					}
				} else if value > maxCode {
					origin, bit = OriginDecode, bitDecode
				}
				switch origin {
				case OriginInput:
					result.InputSamples++
				case OriginDecode:
					result.DecodeSamples++
				default:
					result.InputOrDecodeSamples++
				}
				mark(pl.name, x, y, bit)
				v := Violation{Plane: pl.name, X: x, Y: y, Value: value,
					Low: pl.lo, High: pl.hi, Origin: string(origin)}
				if value < pl.lo {
					v.Delta = value - pl.lo
				} else {
					v.Delta = value - pl.hi
				}
				if len(result.Violations) < maxViolations {
					result.Violations = append(result.Violations, v)
				} else {
					result.ViolationsTruncated = true
				}
			}
		}
	}
	for i, m := range masks {
		if any[i] {
			result.TrueOutOfRangePixels++
		}
		if m&bitInput != 0 {
			result.InputPixels++
		}
		if m&bitDecode != 0 {
			result.DecodePixels++
		}
		if m&bitUnknown != 0 {
			result.InputOrDecodePixels++
		}
	}
	result.FormatAnomalies = decoded.BitDepthAnomaly + decoded.PaddingAnomaly
	return result, nil
}

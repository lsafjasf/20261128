package yuv

type DisplayClipOrigin struct {
	Total          int `json:"total"`
	FromInvalidYUV int `json:"from_invalid_yuv"`
	FromLegalYUV   int `json:"from_legal_yuv"`
}

// ClassifyDisplayClips separates display clipping caused by illegal decoded
// YUV from out-of-gamut values produced by a mathematically legal conversion.
func ClassifyDisplayClips(rgb *RGBFrame, sourceViolation []bool) DisplayClipOrigin {
	var out DisplayClipOrigin
	if len(sourceViolation) != len(rgb.Pix) {
		return out
	}
	for i, p := range rgb.Pix {
		if !p.Clipped {
			continue
		}
		out.Total++
		if sourceViolation[i] {
			out.FromInvalidYUV++
		} else {
			out.FromLegalYUV++
		}
	}
	return out
}

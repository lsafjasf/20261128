package yuv

import (
	"fmt"
)

const ReportSchemaVersion = 1

type FrameInfo struct {
	Width           int          `json:"width"`
	Height          int          `json:"height"`
	BitDepth        int          `json:"bit_depth"`
	SampleFormat    SampleFormat `json:"sample_format"`
	BitDepthAnomaly int          `json:"bit_depth_anomalies"`
	PaddingAnomaly  int          `json:"padding_anomalies"`
}

type ReportSummary struct {
	TrueOutOfRangePixels int    `json:"true_out_of_range_pixels"`
	PrimaryOrigin        string `json:"primary_origin"`
	DisplayClippedPixels int    `json:"display_clipped_pixels"`
	ConversionOnlyPixels int    `json:"conversion_only_clipped_pixels"`
	Conclusion           string `json:"conclusion"`
}

type Report struct {
	SchemaVersion  int               `json:"schema_version"`
	Input          string            `json:"input"`
	Reference      string            `json:"reference,omitempty"`
	Frame          FrameInfo         `json:"frame"`
	Options        ConvertOptions    `json:"options"`
	Samples        SampleAudit       `json:"samples"`
	Classification Classification    `json:"classification"`
	Conversion     ConversionAudit   `json:"conversion"`
	DisplayClips   DisplayClipOrigin `json:"display_clips"`
	Summary        ReportSummary     `json:"summary"`
}

func BuildReport(inputPath, referencePath string, decoded, reference *Frame,
	opts ConvertOptions, maxViolations int) (*Report, error) {
	if decoded == nil {
		return nil, fmt.Errorf("decoded frame is nil")
	}
	if opts.RGBBitDepth < 8 || opts.RGBBitDepth > 16 {
		return nil, fmt.Errorf("unsupported RGB bit depth %d", opts.RGBBitDepth)
	}
	samples, err := InspectSamples(decoded, opts.Range, maxViolations)
	if err != nil {
		return nil, err
	}
	classification, err := ClassifySamples(decoded, reference, opts.Range, maxViolations)
	if err != nil {
		return nil, err
	}
	conversionFrame, conversion, err := ConvertFrame(decoded, opts, maxViolations)
	if err != nil {
		return nil, err
	}
	mask, err := SourceViolationMask(decoded, opts.Range)
	if err != nil {
		return nil, err
	}
	clips := ClassifyDisplayClips(conversionFrame, mask)
	r := &Report{
		SchemaVersion: ReportSchemaVersion,
		Input:         inputPath,
		Reference:     referencePath,
		Frame: FrameInfo{
			Width: decoded.Width, Height: decoded.Height,
			BitDepth: decoded.BitDepth, SampleFormat: decoded.SampleFormat,
			BitDepthAnomaly: decoded.BitDepthAnomaly,
			PaddingAnomaly:  decoded.PaddingAnomaly,
		},
		Options:        opts,
		Samples:        samples,
		Classification: classification,
		Conversion:     conversion,
		DisplayClips:   clips,
	}
	r.Summary = summarize(r)
	return r, nil
}

func summarize(r *Report) ReportSummary {
	s := ReportSummary{
		TrueOutOfRangePixels: r.Classification.TrueOutOfRangePixels,
		DisplayClippedPixels: r.Conversion.OutOfGamutPixels,
		ConversionOnlyPixels: r.DisplayClips.FromLegalYUV,
	}
	c := r.Classification
	switch {
	case c.ReferenceProvided && c.InputSamples > 0 && c.DecodeSamples > 0:
		s.PrimaryOrigin = "mixed_input_and_decode"
	case c.ReferenceProvided && c.InputSamples > 0:
		s.PrimaryOrigin = string(OriginInput)
	case c.ReferenceProvided && c.DecodeSamples > 0:
		s.PrimaryOrigin = string(OriginDecode)
	case !c.ReferenceProvided && c.DecodeSamples > 0 && c.InputOrDecodeSamples > 0:
		s.PrimaryOrigin = "decode_and_unattributed_input"
	case !c.ReferenceProvided && c.DecodeSamples > 0:
		s.PrimaryOrigin = string(OriginDecode)
	case !c.ReferenceProvided && c.InputOrDecodeSamples > 0:
		s.PrimaryOrigin = string(OriginInputOrDecode)
	case c.FormatAnomalies > 0:
		s.PrimaryOrigin = string(OriginDecode)
	case r.DisplayClips.FromLegalYUV > 0:
		s.PrimaryOrigin = "conversion_or_display_clipping"
	default:
		s.PrimaryOrigin = "none"
	}
	switch {
	case s.TrueOutOfRangePixels > 0:
		s.Conclusion = "decoded YUV contains true legal-range code violations; display clipping does not make them disappear"
	case c.FormatAnomalies > 0:
		s.Conclusion = "sample-layout anomalies indicate a decode problem even though no code violation was found"
	case s.ConversionOnlyPixels > 0:
		s.Conclusion = "decoded YUV codes are legal; out-of-range RGB values are display-gamut clipping and must not be counted as YUV overflow"
	default:
		s.Conclusion = "no true out-of-range YUV pixels and no display clipping"
	}
	return s
}

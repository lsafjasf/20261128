package main

import (
	"fmt"
	"io"

	"yuvdiag"
)

func renderText(w io.Writer, r *yuv.Report) {
	yLo, yHi, cLo, cHi, _ := r.Options.Range.Bounds(r.Frame.BitDepth)
	fmt.Fprintln(w, "YUV decode / conversion audit")
	fmt.Fprintf(w, "input: %s\n", r.Input)
	if r.Reference != "" {
		fmt.Fprintf(w, "reference: %s\n", r.Reference)
	}
	fmt.Fprintf(w, "frame: %dx%d, %d-bit, %s\n",
		r.Frame.Width, r.Frame.Height, r.Frame.BitDepth, r.Frame.SampleFormat)
	fmt.Fprintf(w, "range: %s (Y %d..%d, Cb/Cr %d..%d)\n",
		r.Options.Range, yLo, yHi, cLo, cHi)
	fmt.Fprintf(w, "conversion: %s, chroma=%s, RGB=%d-bit\n\n",
		r.Options.Matrix, r.Options.Chroma, r.Options.RGBBitDepth)

	fmt.Fprintln(w, "Raw sample audit (no clipping):")
	renderPlane(w, "Y", r.Samples.Y)
	renderPlane(w, "Cb", r.Samples.Cb)
	renderPlane(w, "Cr", r.Samples.Cr)
	fmt.Fprintf(w, "true out-of-range: %d samples, %d luma-grid pixels\n",
		r.Samples.OutOfRangeSamples, r.Samples.AffectedPixels)
	if r.Frame.BitDepthAnomaly > 0 {
		fmt.Fprintf(w, "  bit-depth anomalies: %d (decoder/sample layout)\n",
			r.Frame.BitDepthAnomaly)
	}
	if r.Frame.PaddingAnomaly > 0 {
		fmt.Fprintf(w, "  P010 padding-bit anomalies: %d (decoder/sample layout)\n",
			r.Frame.PaddingAnomaly)
	}
	fmt.Fprintf(w, "origin: %s\n", r.Summary.PrimaryOrigin)
	if r.Classification.ReferenceProvided {
		fmt.Fprintf(w, "  input: %d samples / %d pixels; decode: %d samples / %d pixels\n",
			r.Classification.InputSamples, r.Classification.InputPixels,
			r.Classification.DecodeSamples, r.Classification.DecodePixels)
	} else if r.Classification.InputOrDecodeSamples > 0 {
		fmt.Fprintln(w, "  no reference frame: in-bit-depth code overflow is input_or_decode")
	}

	fmt.Fprintln(w, "\nCorrected conversion:")
	fmt.Fprintf(w, "  out-of-gamut before display: %d / %d pixels (%d R, %d G, %d B samples)\n",
		r.Conversion.OutOfGamutPixels, r.Conversion.Pixels,
		r.Conversion.OutR, r.Conversion.OutG, r.Conversion.OutB)
	fmt.Fprintf(w, "  display clips: %d from invalid YUV, %d from legal YUV\n",
		r.DisplayClips.FromInvalidYUV, r.DisplayClips.FromLegalYUV)
	fmt.Fprintf(w, "  chroma upsample overshoots: %d\n", r.Conversion.UpsampleOvershoots)
	fmt.Fprintf(w, "conclusion: %s\n", r.Summary.Conclusion)

	if len(r.Classification.Violations) > 0 {
		fmt.Fprintln(w, "\nFirst true sample violations:")
		for _, v := range r.Classification.Violations {
			fmt.Fprintf(w, "  %s (%d,%d): %d outside %d..%d (%+d), origin=%s\n",
				v.Plane, v.X, v.Y, v.Value, v.Low, v.High, v.Delta, v.Origin)
		}
		if r.Classification.ViolationsTruncated {
			fmt.Fprintln(w, "  ... more violations omitted")
		}
	}
	if len(r.Conversion.Violations) > 0 {
		fmt.Fprintln(w, "\nFirst display-clipped pixels (unclipped values):")
		for _, v := range r.Conversion.Violations {
			fmt.Fprintf(w, "  (%d,%d): R=%.6f G=%.6f B=%.6f clip=%s\n",
				v.X, v.Y, v.R, v.G, v.B, v.Clipped)
		}
		if r.Conversion.Truncated {
			fmt.Fprintln(w, "  ... more clipped pixels omitted")
		}
	}
}

func renderPlane(w io.Writer, name string, p yuv.PlaneAudit) {
	fmt.Fprintf(w, "  %-2s %4dx%-4d min=%d max=%d below=%d above=%d\n",
		name, p.Width, p.Height, p.Min, p.Max, p.Below, p.Above)
}

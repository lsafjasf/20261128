package yuv

import "testing"

func reportOptions() ConvertOptions {
	return ConvertOptions{
		Matrix: MatrixBT709, Range: Limited, Chroma: ChromaLeft, RGBBitDepth: 8,
	}
}

func TestBuildReportSeparatesTrueCodesFromDisplayClip(t *testing.T) {
	ref := makeFrame(4, 4, 10, 512, 512, 512)
	decoded := makeFrame(4, 4, 10, 512, 512, 512)
	decoded.Y.Pix[0] = 60
	report, err := BuildReport("decoded.yuv", "reference.yuv", decoded, ref,
		reportOptions(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.TrueOutOfRangePixels != 1 {
		t.Fatalf("true pixels = %d, want 1", report.Summary.TrueOutOfRangePixels)
	}
	if report.Classification.DecodeSamples != 1 || report.Classification.InputSamples != 0 {
		t.Fatalf("classification = %+v", report.Classification)
	}
	if report.DisplayClips.FromInvalidYUV != 1 ||
		report.DisplayClips.FromLegalYUV != report.Conversion.OutOfGamutPixels-1 {
		t.Fatalf("display clips = %+v conversion=%+v", report.DisplayClips, report.Conversion)
	}
}

func TestBuildReportConversionOnly(t *testing.T) {
	f := makeFrame(4, 4, 10, 940, 512, 960)
	report, err := BuildReport("decoded.yuv", "", f, nil, reportOptions(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.TrueOutOfRangePixels != 0 {
		t.Fatalf("legal YUV reported as true overflow: %+v", report.Summary)
	}
	if report.Summary.PrimaryOrigin != "conversion_or_display_clipping" {
		t.Fatalf("origin = %q", report.Summary.PrimaryOrigin)
	}
	if report.Summary.ConversionOnlyPixels != len(f.Y.Pix) {
		t.Fatalf("conversion-only clips = %d, want %d",
			report.Summary.ConversionOnlyPixels, len(f.Y.Pix))
	}
}

package yuv

import "testing"

func convertTestFrame(f *Frame, rng Range) (*RGBFrame, ConversionAudit) {
	opts := ConvertOptions{
		Matrix: MatrixBT709, Range: rng, Chroma: ChromaLeft, RGBBitDepth: 8,
	}
	rgb, audit, err := ConvertFrame(f, opts, 100)
	if err != nil {
		panic(err)
	}
	return rgb, audit
}

func TestConvertLimitedNeutralBoundaries(t *testing.T) {
	for _, tc := range []struct {
		y    uint16
		want uint16
	}{
		{64, 0},
		{940, 255},
	} {
		f := makeFrame(2, 2, 10, tc.y, 512, 512)
		rgb, audit := convertTestFrame(f, Limited)
		for _, p := range rgb.Pix {
			if p.R != tc.want || p.G != tc.want || p.B != tc.want || p.Clipped {
				t.Fatalf("Y=%d pixel = %+v, want %d, audit=%+v", tc.y, p, tc.want, audit)
			}
		}
		if audit.OutOfGamutPixels != 0 || audit.UpsampleOvershoots != 0 {
			t.Fatalf("unexpected audit: %+v", audit)
		}
	}
}

func TestConvertFullRangeNeutralBoundaries(t *testing.T) {
	for _, tc := range []struct {
		y    uint16
		want uint16
	}{
		{0, 0},
		{1023, 255},
	} {
		f := makeFrame(2, 2, 10, tc.y, 512, 512)
		rgb, audit := convertTestFrame(f, Full)
		for _, p := range rgb.Pix {
			if p.R != tc.want || p.G != tc.want || p.B != tc.want || p.Clipped {
				t.Fatalf("full-range Y=%d pixel = %+v, want %d", tc.y, p, tc.want)
			}
		}
		if audit.OutOfGamutPixels != 0 {
			t.Fatalf("full-range neutral Y=%d clipped", tc.y)
		}
	}
}

func TestLegalYUVMayClipAtDisplay(t *testing.T) {
	f := makeFrame(2, 2, 10, 940, 512, 960)
	rgb, audit := convertTestFrame(f, Limited)
	if audit.OutOfGamutPixels != len(rgb.Pix) {
		t.Fatalf("out-of-gamut pixels = %d, want %d", audit.OutOfGamutPixels, len(rgb.Pix))
	}
	mask, err := SourceViolationMask(f, Limited)
	if err != nil {
		t.Fatal(err)
	}
	clips := ClassifyDisplayClips(rgb, mask)
	if clips.FromLegalYUV != len(rgb.Pix) || clips.FromInvalidYUV != 0 {
		t.Fatalf("display clip origin = %+v", clips)
	}
}

func TestInvalidYUVClipIsSeparateFromConversion(t *testing.T) {
	f := makeFrame(2, 2, 10, 60, 512, 512)
	rgb, audit := convertTestFrame(f, Limited)
	mask, err := SourceViolationMask(f, Limited)
	if err != nil {
		t.Fatal(err)
	}
	clips := ClassifyDisplayClips(rgb, mask)
	if audit.OutOfGamutPixels == 0 || clips.FromInvalidYUV != len(rgb.Pix) {
		t.Fatalf("audit=%+v clips=%+v", audit, clips)
	}
}

func TestUpsampleDoesNotAddOverflow(t *testing.T) {
	f := makeFrame(4, 4, 10, 512, 64, 960)
	f.Cb.Pix[0], f.Cb.Pix[1] = 64, 960
	f.Cr.Pix[0], f.Cr.Pix[1] = 960, 64
	_, audit := convertTestFrame(f, Limited)
	if audit.UpsampleOvershoots != 0 {
		t.Fatalf("bilinear upsample overshot %d times", audit.UpsampleOvershoots)
	}
}

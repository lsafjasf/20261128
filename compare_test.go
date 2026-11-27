package yuv

import "testing"

func TestClassifyInputVersusDecode(t *testing.T) {
	ref := makeFrame(4, 4, 10, 512, 512, 512)
	decoded := makeFrame(4, 4, 10, 512, 512, 512)
	ref.Y.Pix[0] = 63
	decoded.Y.Pix[0] = 63
	decoded.Y.Pix[1] = 941
	got, err := ClassifySamples(decoded, ref, Limited, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.InputSamples != 1 || got.DecodeSamples != 1 || got.InputOrDecodeSamples != 0 {
		t.Fatalf("classification = input:%d decode:%d unknown:%d",
			got.InputSamples, got.DecodeSamples, got.InputOrDecodeSamples)
	}
	if got.TrueOutOfRangePixels != 2 {
		t.Fatalf("true pixels = %d, want 2", got.TrueOutOfRangePixels)
	}
}

func TestClassifyWithoutReference(t *testing.T) {
	decoded := makeFrame(4, 4, 10, 512, 512, 512)
	decoded.Y.Pix[0] = 63
	decoded.Y.Pix[1] = 1024
	got, err := ClassifySamples(decoded, nil, Limited, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.InputOrDecodeSamples != 1 || got.DecodeSamples != 1 {
		t.Fatalf("classification = decode:%d unknown:%d", got.DecodeSamples, got.InputOrDecodeSamples)
	}
	if got.DecodedOutOfRangeSamples != 2 || got.TrueOutOfRangePixels != 2 {
		t.Fatalf("counts = samples:%d pixels:%d", got.DecodedOutOfRangeSamples, got.TrueOutOfRangePixels)
	}
}

func TestClassifyRejectsMismatchedReference(t *testing.T) {
	decoded := makeFrame(4, 4, 10, 512, 512, 512)
	ref := makeFrame(2, 2, 10, 512, 512, 512)
	if _, err := ClassifySamples(decoded, ref, Limited, 10); err == nil {
		t.Fatal("expected geometry mismatch error")
	}
}

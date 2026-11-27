package yuv

import "testing"

func TestInspectSamplesCountsRealPixels(t *testing.T) {
	f := makeFrame(4, 4, 10, 512, 512, 512)
	f.Y.Pix[0] = 63
	f.Cb.Pix[1*f.Cb.Stride+1] = 961
	audit, err := InspectSamples(f, Limited, 100)
	if err != nil {
		t.Fatal(err)
	}
	if audit.Y.Below != 1 || audit.Cb.Above != 1 || audit.Cr.OutOfRange != 0 {
		t.Fatalf("plane counts = Y%d/%d Cb%d/%d Cr%d/%d",
			audit.Y.Below, audit.Y.Above, audit.Cb.Below, audit.Cb.Above,
			audit.Cr.Below, audit.Cr.Above)
	}
	if audit.OutOfRangeSamples != 2 {
		t.Fatalf("out-of-range samples = %d, want 2", audit.OutOfRangeSamples)
	}
	if audit.AffectedPixels != 5 {
		t.Fatalf("affected luma-grid pixels = %d, want 5", audit.AffectedPixels)
	}
	if len(audit.Violations) != 2 || audit.Violations[0].Delta != -1 ||
		audit.Violations[1].Delta != 1 {
		t.Fatalf("violations = %+v", audit.Violations)
	}
}

func TestLegalBoundariesAreNotViolations(t *testing.T) {
	f := makeFrame(4, 4, 10, 512, 512, 512)
	for _, v := range []uint16{64, 940} {
		f.Y.Pix[0] = v
		audit, err := InspectSamples(f, Limited, 10)
		if err != nil {
			t.Fatal(err)
		}
		if audit.OutOfRangeSamples != 0 {
			t.Fatalf("Y=%d incorrectly counted", v)
		}
	}
	for _, v := range []uint16{64, 960} {
		f.Cb.Pix[0] = v
		audit, err := InspectSamples(f, Limited, 10)
		if err != nil {
			t.Fatal(err)
		}
		if audit.OutOfRangeSamples != 0 {
			t.Fatalf("Cb=%d incorrectly counted", v)
		}
	}
}

func TestFullRangeDoesNotTreatLimitedExcursionsAsErrors(t *testing.T) {
	f := makeFrame(4, 4, 10, 512, 512, 512)
	f.Y.Pix[0] = 16
	f.Cb.Pix[0] = 960
	audit, err := InspectSamples(f, Full, 10)
	if err != nil {
		t.Fatal(err)
	}
	if audit.OutOfRangeSamples != 0 {
		t.Fatalf("full-range audit reported %d violations", audit.OutOfRangeSamples)
	}
}

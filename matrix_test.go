package yuv

import (
	"math"
	"testing"
)

func TestLimitedNeutralEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name    string
		y       int
		wantRGB float64
	}{
		{"black", 64, 0},
		{"white", 940, 255},
	} {
		t.Run(tc.name, func(t *testing.T) {
			yn, cbn, crn, err := NormalizeCodes(tc.y, 512, 512, 10, Limited)
			if err != nil {
				t.Fatal(err)
			}
			r, g, b, err := MatrixBT709.RGBFromNormalized(yn, cbn, crn)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(r*255-tc.wantRGB) > 1e-9 ||
				math.Abs(g*255-tc.wantRGB) > 1e-9 ||
				math.Abs(b*255-tc.wantRGB) > 1e-9 {
				t.Fatalf("RGB = %.12f %.12f %.12f, want %.12f", r*255, g*255, b*255, tc.wantRGB)
			}
		})
	}
}

func TestQuantizeRGBRoundsOnce(t *testing.T) {
	for _, tc := range []struct {
		in      float64
		want    int
		wantOut bool
	}{
		{0.49, 0, false},
		{0.5, 1, false},
		{1.5, 2, false},
		{254.5, 255, false},
		{-0.49, 0, false},
		{-0.51, 0, true},
		{255.51, 255, true},
		{255.49, 255, false},
	} {
		got, out := QuantizeRGB(tc.in, 8)
		if got != tc.want || out != tc.wantOut {
			t.Fatalf("QuantizeRGB(%v) = %d,%v; want %d,%v",
				tc.in, got, out, tc.want, tc.wantOut)
		}
	}
}

func TestFullRangeNeutral(t *testing.T) {
	yn, cbn, crn, err := NormalizeCodes(1023, 512, 512, 10, Full)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, err := MatrixBT2020.RGBFromNormalized(yn, cbn, crn)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{r, g, b} {
		if math.Abs(v-1) > 1e-9 {
			t.Fatalf("full-range neutral RGB = %v %v %v", r, g, b)
		}
	}
}

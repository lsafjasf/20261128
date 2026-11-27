package raster

import (
	"testing"

	"frameburn/internal/ppm"
)

func TestTextSizeAndPixels(t *testing.T) {
	gotWidth, _ := TextSize("AB", 2)
	if want := (2*GlyphAdvance - 1) * 2; gotWidth != want {
		t.Fatalf("width = %d, want %d", gotWidth, want)
	}
	im, err := ppm.New(32, 16, ppm.Black)
	if err != nil {
		t.Fatal(err)
	}
	DrawText(im, 1, 1, "1", ppm.White, 2)
	nonBlack := 0
	for i := 0; i < len(im.Pix); i += 3 {
		if im.Pix[i] != 0 {
			nonBlack++
		}
	}
	if nonBlack == 0 {
		t.Fatal("text produced no pixels")
	}
}

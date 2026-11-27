package mediaorient

import (
	"image"
	"image/color"
	"testing"
)

func TestRenderAppliesRepeatedRotationExactlyOnce(t *testing.T) {
	source := Source{
		ID: "camera", Width: 4, Height: 2,
		PixelOrientation: OrientRotate90,
		DisplayMatrix:    OrientRotate90.Matrix(4, 2),
		PlayerTransform:  OrientRotate90.Matrix(4, 2),
	}
	raster := testRaster(4, 2)
	frame, err := source.Render(raster, RenderOptions{
		Kind:   OutputProgram,
		Canvas: Size{W: 2, H: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frame.State.Duplicates) != 2 {
		t.Fatalf("duplicates = %+v", frame.State.Duplicates)
	}
	assertRasterPixel(t, frame.Image, 0, 0, raster, 0, 1)
	assertRasterPixel(t, frame.Image, 1, 0, raster, 0, 0)
	assertRasterPixel(t, frame.Image, 0, 3, raster, 3, 1)
	assertRasterPixel(t, frame.Image, 1, 3, raster, 3, 0)
}

func TestRenderDisplayOnlyRotationMatchesProgramAndPreview(t *testing.T) {
	source := Source{
		ID: "display-rotation", Width: 4, Height: 2,
		DisplayMatrix:   OrientRotate90.Matrix(4, 2),
		PlayerTransform: OrientRotate90.Matrix(4, 2),
	}
	raster := testRaster(4, 2)
	preview, err := source.Render(raster, RenderOptions{
		Kind:   OutputPreview,
		Canvas: Size{W: 2, H: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	program, err := source.Render(raster, RenderOptions{
		Kind:   OutputProgram,
		Canvas: Size{W: 2, H: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.State.Orientation != program.State.Orientation {
		t.Fatalf("preview/program orientation differs: %s/%s",
			preview.State.Orientation, program.State.Orientation)
	}
	if !imagesEqual(preview.Image, program.Image) {
		t.Fatal("preview and program pixels differ")
	}
	assertRasterPixel(t, program.Image, 0, 0, raster, 0, 1)
}

func TestRenderUsesCanvasSpaceForCaptions(t *testing.T) {
	source := Source{
		ID: "portrait", Width: 4, Height: 2,
		PixelOrientation: OrientRotate90,
	}
	raster := testRaster(4, 2)
	opts := RenderOptions{
		Canvas:               Size{W: 16, H: 9},
		CaptionInsets:        Insets{Left: 1, Top: 1, Right: 1, Bottom: 1},
		CaptionsWithinActive: true,
	}
	preview, err := source.Render(raster, withKind(opts, OutputPreview))
	if err != nil {
		t.Fatal(err)
	}
	program, err := source.Render(raster, withKind(opts, OutputProgram))
	if err != nil {
		t.Fatal(err)
	}
	p := preview.Geometry.Captions.Safe
	q := program.Geometry.Captions.Safe
	if p != q {
		t.Fatalf("same canvas caption safe area differs: %+v/%+v", p, q)
	}
	if p.W > 14 || p.H > 7 {
		t.Fatalf("caption safe area was cropped more than once: %+v", p)
	}
	if p.X < 1 || p.Y < 1 || p.X+p.W > 15 || p.Y+p.H > 8 {
		t.Fatalf("caption safe area escaped canvas insets: %+v", p)
	}
}

func withKind(opts RenderOptions, kind OutputKind) RenderOptions {
	opts.Kind = kind
	return opts
}

func testRaster(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(10 + x*40),
				G: uint8(20 + y*100),
				B: uint8(30 + x*7 + y*11),
				A: 255,
			})
		}
	}
	return img
}

func assertRasterPixel(
	t *testing.T,
	got *image.NRGBA,
	gx, gy int,
	src *image.NRGBA,
	sx, sy int,
) {
	t.Helper()
	want := src.NRGBAAt(sx, sy)
	if have := got.NRGBAAt(gx, gy); have != want {
		t.Fatalf("pixel (%d,%d) = %+v, want (%d,%d) %+v",
			gx, gy, have, sx, sy, want)
	}
}

func imagesEqual(a, b *image.NRGBA) bool {
	if a.Rect != b.Rect {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

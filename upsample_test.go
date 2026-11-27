package yuv

import "testing"

func TestUpsampleIsConvexAndAligned(t *testing.T) {
	p := Plane{
		Name: "Cb", Width: 2, Height: 2, Stride: 2,
		Pix: []uint16{64, 960, 960, 64},
	}
	for _, loc := range []ChromaLocation{ChromaLeft, ChromaCenter, ChromaTop, ChromaTopLeft} {
		got, err := upsamplePlane(p, 4, 4, loc)
		if err != nil {
			t.Fatal(err)
		}
		for i, v := range got {
			if v < 64 || v > 960 {
				t.Fatalf("%s sample %d = %d overshoots [64,960]", loc, i, v)
			}
		}
	}
}

func TestUpsampleLeftAlignment(t *testing.T) {
	p := Plane{
		Name: "Cb", Width: 2, Height: 1, Stride: 2,
		Pix: []uint16{64, 960},
	}
	got, err := upsamplePlane(p, 4, 1, ChromaLeft)
	if err != nil {
		t.Fatal(err)
	}
	want := []uint16{64, 512, 960, 960}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("x=%d got %d, want %d (all %v)", i, got[i], want[i], got)
		}
	}
}

func TestUpsampleConstantPlane(t *testing.T) {
	p := Plane{
		Name: "Cr", Width: 3, Height: 3, Stride: 3,
		Pix: fillPlane(9, 777),
	}
	for _, loc := range []ChromaLocation{ChromaLeft, ChromaCenter, ChromaTop, ChromaTopLeft} {
		got, err := upsamplePlane(p, 5, 5, loc)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range got {
			if v != 777 {
				t.Fatalf("%s constant plane changed to %d", loc, v)
			}
		}
	}
}

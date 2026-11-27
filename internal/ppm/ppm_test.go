package ppm

import (
	"bytes"
	"testing"
)

func TestPPMRoundTrip(t *testing.T) {
	im, err := New(3, 2, Black)
	if err != nil {
		t.Fatal(err)
	}
	im.Set(1, 0, RGB{12, 34, 56})
	im.FillRect(Rect{X: 0, Y: 1, Width: 2, Height: 1}, RGB{200, 100, 50})

	var buf bytes.Buffer
	if err := im.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String()[:11]; got != "P6\n3 2\n255\n" {
		t.Fatalf("header = %q", got)
	}
	got, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 3 || got.Height != 2 || !bytes.Equal(got.Pix, im.Pix) {
		t.Fatal("round trip changed image")
	}
}

func TestReadPPMComments(t *testing.T) {
	data := []byte("P6 # comment\n2 1\n# another\n255\n" +
		string([]byte{1, 2, 3, 4, 5, 6}))
	im, err := Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if im.At(1, 0) != (RGB{4, 5, 6}) {
		t.Fatalf("pixel = %#v", im.At(1, 0))
	}
}

func TestRectIntersect(t *testing.T) {
	got := (Rect{X: 2, Y: 3, Width: 8, Height: 8}).Intersect(Rect{Width: 5, Height: 6})
	want := Rect{X: 2, Y: 3, Width: 3, Height: 3}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

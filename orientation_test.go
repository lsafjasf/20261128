package mediaorient

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestOrientationEXIFRoundTrip(t *testing.T) {
	for exif := 1; exif <= 8; exif++ {
		got, err := OrientationFromEXIF(exif)
		if err != nil {
			t.Fatalf("OrientationFromEXIF(%d): %v", exif, err)
		}
		if got.EXIF() != exif {
			t.Fatalf("EXIF round trip: got %d, want %d", got.EXIF(), exif)
		}
	}
	if _, err := OrientationFromEXIF(0); !errors.Is(err, ErrInvalidOrientation) {
		t.Fatalf("EXIF 0 error = %v, want ErrInvalidOrientation", err)
	}
}

func TestOrientationJSONUsesEXIFNumbers(t *testing.T) {
	data, err := json.Marshal(OrientRotate90)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "6" {
		t.Fatalf("OrientRotate90 JSON = %s, want 6", data)
	}
	var decoded Orientation
	if err := json.Unmarshal([]byte("8"), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != OrientRotate270 {
		t.Fatalf("EXIF 8 decoded as %s, want %s", decoded, OrientRotate270)
	}
}

func TestOrientationComposition(t *testing.T) {
	tests := []struct {
		name string
		a    Orientation
		b    Orientation
		want Orientation
	}{
		{"two quarter turns", OrientRotate90, OrientRotate90, OrientRotate180},
		{"four quarter turns", OrientRotate90, OrientRotate270, OrientIdentity},
		{"transpose squared", OrientTranspose, OrientTranspose, OrientIdentity},
		{"rotate then flip", OrientRotate90, OrientFlipH, OrientTranspose},
		{"flip then rotate", OrientFlipH, OrientRotate90, OrientTransverse},
		{"identity neutral", OrientIdentity, OrientRotate270, OrientRotate270},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Compose(tt.b); got != tt.want {
				t.Fatalf("%s.Compose(%s) = %s, want %s", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestOrientationMatrixMapsPixels(t *testing.T) {
	const w, h = 4, 2
	tests := []struct {
		orientation Orientation
		from, to    Point
	}{
		{OrientIdentity, Point{X: 0, Y: 0}, Point{X: 0, Y: 0}},
		{OrientFlipH, Point{X: 0, Y: 0}, Point{X: 4, Y: 0}},
		{OrientRotate180, Point{X: 0, Y: 0}, Point{X: 4, Y: 2}},
		{OrientFlipV, Point{X: 0, Y: 0}, Point{X: 0, Y: 2}},
		{OrientTranspose, Point{X: 0, Y: 0}, Point{X: 0, Y: 0}},
		{OrientRotate90, Point{X: 0, Y: 0}, Point{X: 2, Y: 0}},
		{OrientTransverse, Point{X: 0, Y: 0}, Point{X: 2, Y: 4}},
		{OrientRotate270, Point{X: 0, Y: 0}, Point{X: 0, Y: 4}},
	}
	for _, tt := range tests {
		got := tt.orientation.Matrix(w, h).Apply(tt.from)
		if got != tt.to {
			t.Errorf("%s: %v -> %v, want %v", tt.orientation, tt.from, got, tt.to)
		}
		size := tt.orientation.Size(w, h)
		wantSize := Size{W: w, H: h}
		if tt.orientation.SwapsAxes() {
			wantSize = Size{W: h, H: w}
		}
		if size != wantSize {
			t.Errorf("%s: size = %+v, want %+v", tt.orientation, size, wantSize)
		}
	}
}

func TestMatrix3x3AndOrientation(t *testing.T) {
	m, err := Matrix3x3([9]float64{0, -1, 2, 1, 0, 0, 0, 0, 1})
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Orientation(1e-9)
	if err != nil {
		t.Fatal(err)
	}
	if got != OrientRotate90 {
		t.Fatalf("orientation = %s, want %s", got, OrientRotate90)
	}
	scaled := m
	scaled.A, scaled.C = 2, 2
	if _, err := scaled.Orientation(1e-9); !errors.Is(err, ErrUnsupportedMatrix) {
		t.Fatalf("scaled matrix error = %v, want ErrUnsupportedMatrix", err)
	}
}

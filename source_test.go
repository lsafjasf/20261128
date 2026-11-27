package ghostdiag

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"ghostdiag/internal/synth"
)

func TestY4MSourceRoundTrip(t *testing.T) {
	seq := synth.Build()
	var encoded bytes.Buffer
	if err := synth.WriteY4M(&encoded, seq, seq.Original[:2]); err != nil {
		t.Fatal(err)
	}
	source, err := NewY4MSource(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		frame, err := source.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(frame.Plane.Pix, seq.Original[i]) {
			t.Fatalf("frame %d luma differs", i)
		}
	}
	if _, err := source.Next(); !errors.Is(err, ErrEndOfStream) {
		t.Fatalf("end error=%v", err)
	}
}

func TestRawSource(t *testing.T) {
	const width, height = 8, 4
	var encoded bytes.Buffer
	for frame := 0; frame < 2; frame++ {
		encoded.Write(bytes.Repeat([]byte{byte(40 + frame)}, width*height))
	}
	source, err := NewRawSource(&encoded, RawConfig{
		Width: width, Height: height, FrameRate: 10, PixelFormat: "gray",
	})
	if err != nil {
		t.Fatal(err)
	}
	for frame := 0; frame < 2; frame++ {
		got, err := source.Next()
		if err != nil {
			t.Fatal(err)
		}
		if got.Plane.Pix[0] != byte(40+frame) {
			t.Fatalf("frame %d value=%d", frame, got.Plane.Pix[0])
		}
	}
}

func TestImageSourceNaturalOrder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"frame-10.png", "frame-2.png"} {
		value := uint8(20)
		if name == "frame-2.png" {
			value = 200
		}
		img := image.NewGray(image.Rect(0, 0, 4, 4))
		for i := range img.Pix {
			img.Pix[i] = value
		}
		file, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, img); err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
	}
	source, err := NewImageSource(dir, 12)
	if err != nil {
		t.Fatal(err)
	}
	first, err := source.Next()
	if err != nil {
		t.Fatal(err)
	}
	if first.Plane.Pix[0] != 200 {
		t.Fatalf("natural order picked %q first", source.paths[0])
	}
}

func TestLumaImageConversion(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	if got := FromImage(img).Pix[0]; got == 0 {
		t.Fatal("red channel produced zero luma")
	}
}

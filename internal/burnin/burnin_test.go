package burnin

import (
	"bytes"
	"testing"

	"frameburn/internal/ppm"
	"frameburn/internal/timecode"
)

func TestStampUsesOutputIndex(t *testing.T) {
	df := timecode.Rate2997DF()
	stamp, err := NewStamp(df, 1800)
	if err != nil {
		t.Fatal(err)
	}
	if stamp.Timecode != "00:01:00;02" || stamp.FrameTag != "001800" {
		t.Fatalf("unexpected stamp: %#v", stamp)
	}
	nd, err := NewStamp(timecode.Rate23976(), 1440)
	if err != nil {
		t.Fatal(err)
	}
	if nd.Timecode != "00:01:00:00" {
		t.Fatalf("unexpected 23.976 stamp: %#v", nd)
	}
}

func TestBurnDoesNotDependOnPicture(t *testing.T) {
	styleScale := 1
	a, _ := ppm.New(240, 120, ppm.RGB{R: 20, G: 80, B: 120})
	b, _ := ppm.New(240, 120, ppm.RGB{R: 210, G: 40, B: 10})
	stamp, _ := NewStamp(timecode.Rate2997DF(), 1800)
	style := DefaultStyle(a)
	style.Scale = styleScale
	style.Padding = 2
	panelA := Burn(a, stamp, style)
	panelB := Burn(b, stamp, style)
	if panelA != panelB {
		t.Fatalf("panel moved: %#v vs %#v", panelA, panelB)
	}
	content := style.ContentRect(a, stamp.Label)
	for y := panelA.Y; y < panelA.Y+panelA.Height; y++ {
		for x := panelA.X; x < panelA.X+panelA.Width; x++ {
			if a.At(x, y) != b.At(x, y) {
				t.Fatalf("stamp pixel differs at %d,%d", x, y)
			}
		}
	}
	if content.Empty() {
		t.Fatal("content rectangle should not be empty")
	}
}

func TestStampLabelContainsBothFields(t *testing.T) {
	stamp, err := NewStamp(timecode.Rate23976(), 23)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(stamp.Label), []byte("000023")) ||
		!bytes.Contains([]byte(stamp.Label), []byte("00:00:00:23")) {
		t.Fatalf("label = %q", stamp.Label)
	}
}

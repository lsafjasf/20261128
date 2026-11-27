package fontmetrics

import (
	"os"
	"testing"
)

func TestLoadSystemFont(t *testing.T) {
	path := "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
	if _, err := os.Stat(path); err != nil {
		t.Skip("DejaVu Sans is not installed")
	}
	face, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	width, ok := face.Advance('A')
	if !ok || width <= 0 {
		t.Fatalf("advance('A') = %v, %v", width, ok)
	}
	if face.Ascent() <= 0 || face.Descent() <= 0 {
		t.Fatalf("invalid vertical metrics: %v/%v", face.Ascent(), face.Descent())
	}
}

func TestLoadSystemCJKFallback(t *testing.T) {
	path := "/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf"
	if _, err := os.Stat(path); err != nil {
		t.Skip("Droid Sans Fallback is not installed")
	}
	face, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	width, ok := face.Advance('漢')
	if !ok || width <= 0 {
		t.Fatalf("advance('漢') = %v, %v", width, ok)
	}
}

package mediaorient

import (
	"strings"
	"testing"
)

func TestJointStateNeutralizesDuplicateRotation(t *testing.T) {
	const w, h = 4000, 3000
	source := Source{
		ID:               "camera-a",
		Width:            w,
		Height:           h,
		PixelOrientation: OrientRotate90,
		DisplayMatrix:    OrientRotate90.Matrix(w, h),
		PlayerTransform:  OrientRotate90.Matrix(w, h),
	}
	state, err := source.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if state.Orientation != OrientRotate90 {
		t.Fatalf("orientation = %s, want %s", state.Orientation, OrientRotate90)
	}
	if state.Upright != (Size{W: 3000, H: 4000}) {
		t.Fatalf("upright = %+v, want 3000x4000", state.Upright)
	}
	if len(state.Duplicates) != 2 {
		t.Fatalf("duplicates = %+v, want 2", state.Duplicates)
	}
	assertDuplicate(t, state.Duplicates, LayerDisplay, LayerPixels)
	assertDuplicate(t, state.Duplicates, LayerPlayer, LayerPixels)
	if state.Layers[0].Applied != OrientRotate90 {
		t.Fatalf("pixel layer applied = %s", state.Layers[0].Applied)
	}
	if state.Layers[1].Applied != OrientIdentity || state.Layers[2].Applied != OrientIdentity {
		t.Fatalf("duplicate layers still applied: %+v", state.Layers)
	}
	if !strings.Contains(state.DuplicateSummary(), "display-matrix repeats rotate-90-cw") {
		t.Fatalf("summary does not identify duplicate display rotation: %s", state.DuplicateSummary())
	}
}

func TestDisplayOwnsRotationWhenPixelsAreUpright(t *testing.T) {
	source := Source{
		ID:              "file-b",
		Width:           1920,
		Height:          1080,
		DisplayMatrix:   OrientRotate270.Matrix(1920, 1080),
		PlayerTransform: OrientRotate270.Matrix(1920, 1080),
	}
	state, err := source.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if state.Orientation != OrientRotate270 {
		t.Fatalf("orientation = %s, want rotate-270-cw", state.Orientation)
	}
	if len(state.Duplicates) != 1 {
		t.Fatalf("duplicates = %+v, want one", state.Duplicates)
	}
	assertDuplicate(t, state.Duplicates, LayerPlayer, LayerDisplay)
}

func TestDistinctLayersComposeWithoutFalseDuplicate(t *testing.T) {
	const w, h = 640, 480
	source := Source{
		ID:               "intentional-adjustment",
		Width:            w,
		Height:           h,
		PixelOrientation: OrientRotate90,
		DisplayMatrix:    OrientRotate180.Matrix(w, h),
		PlayerTransform:  OrientRotate270.Matrix(w, h),
	}
	state, err := source.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Duplicates) != 0 {
		t.Fatalf("unexpected duplicate diagnostics: %+v", state.Duplicates)
	}
	if state.Orientation != OrientRotate180 {
		t.Fatalf("orientation = %s, want rotate-180", state.Orientation)
	}
}

func assertDuplicate(t *testing.T, duplicates []Duplicate, layer, kept Layer) {
	t.Helper()
	for _, duplicate := range duplicates {
		if duplicate.Layer == layer && duplicate.KeptLayer == kept {
			return
		}
	}
	t.Fatalf("duplicate %s owned by %s not found in %+v", layer, kept, duplicates)
}

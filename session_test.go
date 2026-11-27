package mediaorient

import (
	"math"
	"testing"
)

func TestSessionKeepsEverySourceConsistentAcrossBuses(t *testing.T) {
	session, err := NewSession(SessionConfig{
		PreviewCanvas:        Size{W: 640, H: 360},
		ProgramCanvas:        Size{W: 1920, H: 1080},
		Fit:                  FitContain,
		CaptionInsets:        Insets{Left: 96, Top: 32, Right: 96, Bottom: 64},
		CaptionsWithinActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sources := []Source{
		{
			ID: "portrait", Width: 4000, Height: 3000,
			PixelOrientation: OrientRotate90,
			DisplayMatrix:    OrientRotate90.Matrix(4000, 3000),
			PlayerTransform:  OrientRotate90.Matrix(4000, 3000),
		},
		{
			ID: "landscape", Width: 1920, Height: 1080,
		},
		{
			ID: "transposed", Width: 3000, Height: 4000,
			PixelOrientation: OrientTranspose,
		},
	}
	for _, source := range sources {
		if err := session.AddSource(source); err != nil {
			t.Fatalf("AddSource(%s): %v", source.ID, err)
		}
	}
	if err := session.Cut("portrait"); err != nil {
		t.Fatal(err)
	}

	snapshot, err := session.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.PreviewID != "portrait" || snapshot.ProgramID != "portrait" {
		t.Fatalf("active buses = %s/%s", snapshot.PreviewID, snapshot.ProgramID)
	}
	for id, view := range snapshot.Sources {
		if view.State.Orientation != mustResolve(t, view.Source).Orientation {
			t.Errorf("%s: snapshot state differs from source resolution", id)
		}
		if view.Preview.Upright != view.Program.Upright {
			t.Errorf("%s: upright size differs preview/program: %+v/%+v",
				id, view.Preview.Upright, view.Program.Upright)
		}
		assertCaptionSafeNormalized(t, id, view)
	}

	if err := session.SwitchProgram("landscape"); err != nil {
		t.Fatal(err)
	}
	if err := session.SwitchPreview("transposed"); err != nil {
		t.Fatal(err)
	}
	snapshot, err = session.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.PreviewID != "transposed" || snapshot.ProgramID != "landscape" {
		t.Fatalf("active buses after separate switches = %s/%s",
			snapshot.PreviewID, snapshot.ProgramID)
	}
}

func TestSessionRejectsUnknownSwitchAtomically(t *testing.T) {
	session, err := NewSession(SessionConfig{
		PreviewCanvas: Size{W: 32, H: 18},
		ProgramCanvas: Size{W: 64, H: 36},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.AddSource(Source{ID: "a", Width: 4, Height: 3}); err != nil {
		t.Fatal(err)
	}
	if err := session.Cut("missing"); err == nil {
		t.Fatal("Cut accepted an unknown source")
	}
	snapshot, err := session.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.PreviewID != "a" || snapshot.ProgramID != "a" {
		t.Fatalf("failed switch changed buses: %s/%s", snapshot.PreviewID, snapshot.ProgramID)
	}
}

func mustResolve(t *testing.T, source Source) JointState {
	t.Helper()
	state, err := source.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func assertCaptionSafeNormalized(t *testing.T, id string, view SourceView) {
	t.Helper()
	p := view.Preview.Captions.NormalizedSafe
	q := view.Program.Captions.NormalizedSafe
	if math.Abs(p.X-q.X) > 1e-9 || math.Abs(p.Y-q.Y) > 1e-9 ||
		math.Abs(p.W-q.W) > 1e-9 || math.Abs(p.H-q.H) > 1e-9 {
		t.Errorf("%s: caption safe area differs: %+v vs %+v", id, p, q)
	}
}

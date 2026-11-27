package timeline

import (
	"testing"

	"mediadedup/dedup"
	"mediadedup/media"
)

func testPool() *dedup.Pool {
	assets := []media.Asset{
		{ID: "A1", Duration: 12, FrameRate: 30, Rotation: media.Rotate0, Codec: "prores",
			Width: 1920, Height: 1080, BitrateK: 45000,
			Video: media.VideoFingerprint{Frames: []uint64{1, 2, 3}},
			Audio: media.AudioFingerprint{Points: []uint64{9, 8, 7}}},
		{ID: "A2", Duration: 12, FrameRate: 24, Rotation: media.Rotate90, Codec: "h264",
			Width: 1920, Height: 1080, BitrateK: 8000,
			Video: media.VideoFingerprint{Frames: []uint64{1, 2, 3}},
			Audio: media.AudioFingerprint{Points: []uint64{9, 8, 7}}},
		{ID: "B1", Duration: 12, FrameRate: 30, Rotation: media.Rotate0, Codec: "prores",
			Width: 1920, Height: 1080, BitrateK: 40000,
			Video: media.VideoFingerprint{Frames: []uint64{0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFE, 0xFFFFFFFFFFFFFFFD}},
			Audio: media.AudioFingerprint{Points: []uint64{0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFE, 0xFFFFFFFFFFFFFFFD}}},
		{ID: "S1", Duration: 30, FrameRate: 30, Rotation: media.Rotate0, Codec: "h264",
			Width: 1920, Height: 1080, BitrateK: 5000,
			Video: media.VideoFingerprint{Frames: []uint64{0xAAAAAAAAAAAAAAAA}},
			Audio: media.AudioFingerprint{Points: []uint64{0xAAAAAAAAAAAAAAAA}}},
	}
	return dedup.Build(assets, dedup.DefaultThresholds())
}

func findTestGroup(t *testing.T, pool *dedup.Pool, assetID string) *dedup.Group {
	t.Helper()
	g, ok := pool.GroupOf(assetID)
	if !ok {
		t.Fatalf("no group for %s", assetID)
	}
	return g
}

func TestReplaceGroupScoped(t *testing.T) {
	pool := testPool()
	g := findTestGroup(t, pool, "A1")
	if err := g.SetMaster("A1"); err != nil {
		t.Fatal(err)
	}

	tl := &Timeline{Clips: []Clip{
		{ID: "c1", TrackID: "V1", AssetID: "A2", Start: 0, SourceIn: 1, SourceOut: 5},
		{ID: "c2", TrackID: "V1", AssetID: "B1", Start: 6, SourceIn: 0, SourceOut: 4},
		{ID: "c3", TrackID: "V2", AssetID: "S1", Start: 12, SourceIn: 2, SourceOut: 8},
		{ID: "c4", TrackID: "V1", AssetID: "A1", Start: 20, SourceIn: 0, SourceOut: 3},
	}}

	report, err := ReplaceWithMaster(tl, pool, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Replacements) != 1 {
		t.Fatalf("replacements=%d want 1", len(report.Replacements))
	}
	rep := report.Replacements[0]
	if rep.ClipID != "c1" || rep.FromAssetID != "A2" || rep.ToAssetID != "A1" {
		t.Fatalf("unexpected replacement: %+v", rep)
	}
	if rep.RotationFix != media.Rotate90 {
		t.Fatalf("rotation fix=%d want 90", rep.RotationFix)
	}
	if rep.FrameRateScale != 24.0/30.0 {
		t.Fatalf("rate scale=%v want %v", rep.FrameRateScale, 24.0/30.0)
	}

	// Other groups and singletons are untouched.
	if tl.Clips[1].AssetID != "B1" {
		t.Fatalf("other-group clip rewritten to %s", tl.Clips[1].AssetID)
	}
	if tl.Clips[2].AssetID != "S1" {
		t.Fatalf("singleton clip rewritten to %s", tl.Clips[2].AssetID)
	}
	// Timing preserved.
	if tl.Clips[0].Start != 0 || tl.Clips[0].SourceIn != 1 || tl.Clips[0].SourceOut != 5 {
		t.Fatal("clip timing must be preserved")
	}
	// Already-master clip is skipped.
	if len(report.Skipped) != 1 || report.Skipped[0] != "c4" {
		t.Fatalf("skipped=%v want [c4]", report.Skipped)
	}
}

func TestReplaceIdempotent(t *testing.T) {
	pool := testPool()
	g := findTestGroup(t, pool, "A1")
	if err := g.SetMaster("A1"); err != nil {
		t.Fatal(err)
	}
	tl := &Timeline{Clips: []Clip{
		{ID: "c1", AssetID: "A2", Start: 0, SourceIn: 0, SourceOut: 2},
	}}

	if _, err := ReplaceWithMaster(tl, pool, g.ID); err != nil {
		t.Fatal(err)
	}
	report, err := ReplaceWithMaster(tl, pool, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Replacements) != 0 || len(report.Skipped) != 1 {
		t.Fatalf("second run must be no-op: %+v", report)
	}
	if tl.Clips[0].AssetID != "A1" {
		t.Fatalf("asset=%s want A1", tl.Clips[0].AssetID)
	}
}

func TestReplaceUnknownGroup(t *testing.T) {
	pool := testPool()
	tl := &Timeline{}
	if _, err := ReplaceWithMaster(tl, pool, "G999"); err == nil {
		t.Fatal("unknown group must error")
	}
}

func TestRotationCompensation(t *testing.T) {
	cases := []struct {
		from, to, want media.Rotation
	}{
		{media.Rotate90, media.Rotate0, media.Rotate90},
		{media.Rotate0, media.Rotate90, media.Rotate270},
		{media.Rotate180, media.Rotate90, media.Rotate90},
		{media.Rotate0, media.Rotate0, media.Rotate0},
	}
	for _, c := range cases {
		if got := rotationCompensation(c.from, c.to); got != c.want {
			t.Errorf("rotationCompensation(%d,%d)=%d want %d", c.from, c.to, got, c.want)
		}
	}
}

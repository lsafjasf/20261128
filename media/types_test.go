package media

import "testing"

func TestRotationSwapped(t *testing.T) {
	cases := map[Rotation]bool{
		Rotate0: false, Rotate90: true, Rotate180: false, Rotate270: true,
		Rotation(450): true, Rotation(-90): true,
	}
	for r, want := range cases {
		if got := r.Normalize().Swapped(); got != want {
			t.Errorf("rotation %d Swapped=%v want %v", r, got, want)
		}
	}
}

func TestUprightDimensions(t *testing.T) {
	a := Asset{Width: 1920, Height: 1080, Rotation: Rotate90}
	if w, h := a.UprightWidth(), a.UprightHeight(); w != 1080 || h != 1920 {
		t.Fatalf("upright dims = %dx%d, want 1080x1920", w, h)
	}
	b := Asset{Width: 1920, Height: 1080}
	if w, h := b.UprightWidth(), b.UprightHeight(); w != 1920 || h != 1080 {
		t.Fatalf("upright dims = %dx%d, want 1920x1080", w, h)
	}
}

func TestVideoDistance(t *testing.T) {
	a := VideoFingerprint{Frames: []uint64{0, 0, 0}}
	b := VideoFingerprint{Frames: []uint64{1, 2, 4}} // 1,1,1 set bits
	got := VideoDistance(a, b)
	// 3 differing bits total / (3 words * 64)
	want := 3.0 / (3 * 64)
	if d := got - want; d > 1e-12 || d < -1e-12 {
		t.Fatalf("VideoDistance=%v want %v", got, want)
	}
}

func TestVideoDistanceEmpty(t *testing.T) {
	if got := VideoDistance(VideoFingerprint{}, VideoFingerprint{Frames: []uint64{1}}); got != 1.0 {
		t.Fatalf("empty fingerprint distance=%v want 1.0", got)
	}
}

func TestResampleHashes(t *testing.T) {
	src := []uint64{10, 20, 30, 40, 50}
	got := resampleHashes(src, 3)
	want := []uint64{10, 30, 50}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resample[%d]=%d want %d (full=%v)", i, got[i], want[i], got)
		}
	}
	// Identity case.
	again := resampleHashes(src, 5)
	for i := range src {
		if again[i] != src[i] {
			t.Fatalf("identity resample changed index %d", i)
		}
	}
}

func TestSameFile(t *testing.T) {
	a := Asset{ID: "a", FileHash: []byte{1, 2, 3}}
	b := Asset{ID: "b", FileHash: []byte{1, 2, 3}}
	c := Asset{ID: "c", FileHash: []byte{9, 9, 9}}
	d := Asset{ID: "d"}
	if !SameFile(a, b) {
		t.Fatal("equal hashes should be same file")
	}
	if SameFile(a, c) {
		t.Fatal("different hashes must not match")
	}
	if SameFile(a, d) {
		t.Fatal("missing hash must never match")
	}
}

func TestAudioDistanceResampled(t *testing.T) {
	// Different bucket counts but identical content: nearest-neighbor
	// resampling should drive distance to zero.
	a := AudioFingerprint{Points: []uint64{7, 7, 7, 7}}
	b := AudioFingerprint{Points: []uint64{7, 7}}
	if got := AudioDistance(a, b); got != 0 {
		t.Fatalf("resampled identical audio distance=%v want 0", got)
	}
}

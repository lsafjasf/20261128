package dedup

import (
	"testing"

	"mediadedup/media"
)

const baseHash uint64 = 0x0F0F0F0F0F0F0F0F

// noisyHash flips up to n low bits to emulate re-encode hash jitter.
func noisyHash(h uint64, n uint) uint64 {
	var mask uint64 = (1 << n) - 1
	return h ^ mask
}

func frames(h uint64, n int, noise uint) []uint64 {
	out := make([]uint64, n)
	for i := range out {
		out[i] = noisyHash(h+uint64(i)*0x1111111111111111, noise)
	}
	return out
}

func audioPoints(h uint64, n int, noise uint) []uint64 {
	out := make([]uint64, n)
	for i := range out {
		out[i] = noisyHash(h+uint64(i)*0x0123456789ABCDEF, noise)
	}
	return out
}

func asset(id string, dur, fps float64, rot media.Rotation, codec string, bitrate int, vh, ah uint64, nf int) media.Asset {
	w, h := 1920, 1080
	if rot.Swapped() {
		w, h = 1080, 1920
	}
	return media.Asset{
		ID: id, Path: "/pool/" + id + ".mov", Duration: dur,
		Width: w, Height: h, FrameRate: fps, Rotation: rot,
		Codec: codec, BitrateK: bitrate,
		Video: media.VideoFingerprint{Frames: frames(vh, nf, 0), Rotation: rot},
		Audio: media.AudioFingerprint{Points: audioPoints(ah, nf, 0)},
	}
}

func noisyAsset(id string, dur, fps float64, rot media.Rotation, codec string, bitrate int, vh, ah uint64, nf int, noise uint) media.Asset {
	a := asset(id, dur, fps, rot, codec, bitrate, vh, ah, nf)
	for i := range a.Video.Frames {
		a.Video.Frames[i] = noisyHash(a.Video.Frames[i], noise)
	}
	for i := range a.Audio.Points {
		a.Audio.Points[i] = noisyHash(a.Audio.Points[i], noise)
	}
	return a
}

func poolFixtures() []media.Asset {
	return []media.Asset{
		// Group 1: shot A, three representations.
		asset("A1", 12.0, 30, media.Rotate0, "prores", 45000, baseHash, baseHash, 12),
		asset("A2", 12.0, 24, media.Rotate90, "h264", 8000, baseHash, baseHash, 12),
		noisyAsset("A3", 12.03, 60, media.Rotate0, "h265", 12000, baseHash, baseHash, 12, 3),
		// Group 2: shot B, two representations.
		asset("B1", 12.0, 30, media.Rotate0, "prores", 40000, baseHash^0xFFFFFFFFFFFFFFFF, baseHash^0xFFFFFFFFFFFFFFFF, 12),
		noisyAsset("B2", 12.0, 30, media.Rotate180, "h264", 6000, baseHash^0xFFFFFFFFFFFFFFFF, baseHash^0xFFFFFFFFFFFFFFFF, 12, 2),
		// Singletons: different duration / different shot.
		asset("C1", 30.0, 30, media.Rotate0, "h264", 5000, 0xAAAAAAAAAAAAAAAA, 0xAAAAAAAAAAAAAAAA, 12),
		asset("D1", 12.0, 30, media.Rotate0, "h264", 5000, 0x5555555555555555, 0x5555555555555555, 12),
	}
}

func TestClassifyIdentical(t *testing.T) {
	t1 := DefaultThresholds()
	a := asset("x", 10, 30, 0, "h264", 5000, baseHash, baseHash, 8)
	b := asset("y", 10, 30, 0, "h264", 5000, baseHash, baseHash, 8)
	r := ClassifyPair(a, b, t1)
	if r.Relation != RelationIdentical {
		t.Fatalf("relation=%s want identical (reasons=%v)", r.Relation, r.Reasons)
	}
}

func TestClassifyIdenticalByFileHash(t *testing.T) {
	t1 := DefaultThresholds()
	a := asset("x", 10, 24, 0, "h264", 5000, baseHash, baseHash, 8)
	b := asset("y", 10, 30, media.Rotate90, "prores", 40000, baseHash, baseHash, 8)
	a.FileHash = []byte{0xAB, 0xCD}
	b.FileHash = []byte{0xAB, 0xCD}
	r := ClassifyPair(a, b, t1)
	if r.Relation != RelationIdentical {
		t.Fatalf("relation=%s want identical via file hash", r.Relation)
	}
}

func TestClassifyRotationOnlyIsIdentical(t *testing.T) {
	t1 := DefaultThresholds()
	a := asset("x", 10, 30, 0, "h264", 5000, baseHash, baseHash, 8)
	b := asset("y", 10, 30, media.Rotate90, "h264", 5000, baseHash, baseHash, 8)
	r := ClassifyPair(a, b, t1)
	if r.Relation != RelationIdentical {
		t.Fatalf("relation=%s want identical, reasons=%v", r.Relation, r.Reasons)
	}
}

func TestClassifyTranscode(t *testing.T) {
	t1 := DefaultThresholds()
	a := asset("x", 10, 24, 0, "h264", 5000, baseHash, baseHash, 8)
	b := noisyAsset("y", 10.05, 60, media.Rotate180, "h265", 12000, baseHash, baseHash, 8, 3)
	r := ClassifyPair(a, b, t1)
	if r.Relation != RelationTranscode {
		t.Fatalf("relation=%s want transcode (vd=%.3f ad=%.3f reasons=%v)",
			r.Relation, r.VideoDistance, r.AudioDistance, r.Reasons)
	}
}

func TestClassifyDifferentShot(t *testing.T) {
	t1 := DefaultThresholds()
	a := asset("x", 10, 30, 0, "h264", 5000, baseHash, baseHash, 8)
	b := asset("y", 10, 30, 0, "h264", 5000, 0xDEADBEEF, 0xDEADBEEF, 8)
	r := ClassifyPair(a, b, t1)
	if r.Relation != RelationDifferent {
		t.Fatalf("relation=%s want different", r.Relation)
	}
}

func TestClassifyDurationGap(t *testing.T) {
	t1 := DefaultThresholds()
	a := asset("x", 10, 30, 0, "h264", 5000, baseHash, baseHash, 8)
	b := asset("y", 12, 30, 0, "h264", 5000, baseHash, baseHash, 8)
	r := ClassifyPair(a, b, t1)
	if r.Relation != RelationDifferent || len(r.Reasons) == 0 || r.Reasons[0] != ReasonDurationGap {
		t.Fatalf("relation=%s reasons=%v want duration-gap different", r.Relation, r.Reasons)
	}
}

func TestClassifyVideoOnlyMismatch(t *testing.T) {
	t1 := DefaultThresholds()
	// Same audio bed, different picture (e.g. replaced B-roll): must stay split.
	a := asset("x", 10, 30, 0, "h264", 5000, baseHash, baseHash, 8)
	b := asset("y", 10, 30, 0, "h264", 5000, 0xFFFFFFFFFFFFFFFF, baseHash, 8)
	r := ClassifyPair(a, b, t1)
	if r.Relation != RelationDifferent {
		t.Fatalf("relation=%s want different when video alone mismatches", r.Relation)
	}
}

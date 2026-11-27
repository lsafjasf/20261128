// Package media defines pool asset metadata and the fingerprints used to
// compare assets without touching the underlying media files.
//
// A real ingestion pipeline would populate these structures from ffprobe /
// decoder output; here the data model is deliberately self-contained so the
// comparison logic can be unit tested deterministically.
package media

import (
	"bytes"
	"math"
)

// Rotation describes the stored rotation metadata of an asset, in degrees.
type Rotation int

const (
	Rotate0   Rotation = 0
	Rotate90  Rotation = 90
	Rotate180 Rotation = 180
	Rotate270 Rotation = 270
)

// Swapped reports whether this rotation swaps the width/height axes.
func (r Rotation) Swapped() bool {
	return r == Rotate90 || r == Rotate270
}

// Normalize maps any rotation onto {0, 90, 180, 270}.
func (r Rotation) Normalize() Rotation {
	angle := ((int(r) % 360) + 360) % 360
	return Rotation(angle)
}

// VideoFingerprint is a rotation-aware sampled-frame fingerprint.
//
// Frames hold one perceptual hash (64-bit, e.g. an aHash/dHash/pHash value)
// per sampled frame, ordered by display time after rotation normalization:
// frame hashes are computed on the upright image, so the same shot captured
// with different rotation metadata produces comparable hashes.
type VideoFingerprint struct {
	Frames   []uint64
	Rotation Rotation
}

// AudioFingerprint is a sampled audio fingerprint.
//
// Points hold one 64-bit landmark hash per fixed audio time bucket. Two clips
// sampled at the same effective rate align bucket-for-bucket; resampling noise
// is tolerated by the hamming thresholds rather than explicit alignment.
type AudioFingerprint struct {
	Points []uint64
}

// Asset is a single media-pool entry.
type Asset struct {
	ID        string
	Path      string
	Duration  float64 // seconds
	Width     int
	Height    int
	FrameRate float64 // frames per second
	Rotation  Rotation
	Codec     string
	BitrateK  int
	// FileHash is an optional whole-file content hash. When present and equal
	// it proves byte-identical assets even before fingerprint comparison.
	FileHash []byte
	Video    VideoFingerprint
	Audio    AudioFingerprint
}

// UprightWidth returns the visible width after rotation metadata is applied.
func (a Asset) UprightWidth() int {
	if a.Rotation.Swapped() {
		return a.Height
	}
	return a.Width
}

// UprightHeight returns the visible height after rotation metadata is applied.
func (a Asset) UprightHeight() int {
	if a.Rotation.Swapped() {
		return a.Width
	}
	return a.Height
}

// SameFile reports whether two assets are byte-identical based on file hashes.
// Missing hashes never produce a positive match.
func SameFile(a, b Asset) bool {
	return len(a.FileHash) > 0 && bytes.Equal(a.FileHash, b.FileHash)
}

// popcount counts the set bits of a 64-bit word.
func popcount(x uint64) int {
	// math/bits.OnesCount64 would do, but keeping the expression local makes
	// the distance code self-explanatory.
	x = x - ((x >> 1) & 0x5555555555555555)
	x = (x & 0x3333333333333333) + ((x >> 2) & 0x3333333333333333)
	x = (x + (x >> 4)) & 0x0f0f0f0f0f0f0f0f
	return int((x * 0x0101010101010101) >> 56)
}

// hamming64 returns the normalized hamming distance in [0,1] between two words.
func hamming64(a, b uint64) float64 {
	return float64(popcount(a^b)) / 64.0
}

// resampleHashes projects n source hashes onto m output positions using
// nearest-neighbor indexing, so fingerprints with different sample counts
// (different frame rates) can be compared on a common timeline.
func resampleHashes(src []uint64, m int) []uint64 {
	if m <= 0 {
		return nil
	}
	n := len(src)
	if n == m {
		out := make([]uint64, n)
		copy(out, src)
		return out
	}
	out := make([]uint64, m)
	for i := 0; i < m; i++ {
		idx := int(math.Round(float64(i) * float64(n-1) / float64(m-1)))
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		out[i] = src[idx]
	}
	return out
}

// meanHamming returns the mean normalized hamming distance of two hash
// sequences after aligning their lengths. An empty sequence yields 1.0
// (maximally different) rather than a false zero-distance match.
func meanHamming(a, b []uint64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 1.0
	}
	if len(a) != len(b) {
		if len(b) < len(a) {
			a, b = b, a
		}
		a = resampleHashes(a, len(b))
	}
	var sum float64
	for i := range a {
		sum += hamming64(a[i], b[i])
	}
	return sum / float64(len(a))
}

// VideoDistance returns the mean normalized perceptual-hash distance of two
// video fingerprints in [0,1]. Hashes are already upright, so rotation only
// matters indirectly (a rotated transcode may carry slight re-encode noise).
func VideoDistance(a, b VideoFingerprint) float64 {
	return meanHamming(a.Frames, b.Frames)
}

// AudioDistance returns the mean normalized fingerprint distance of two audio
// tracks in [0,1].
func AudioDistance(a, b AudioFingerprint) float64 {
	return meanHamming(a.Points, b.Points)
}

// DurationDistance returns the absolute duration gap in seconds.
func DurationDistance(a, b Asset) float64 {
	d := a.Duration - b.Duration
	if d < 0 {
		return -d
	}
	return d
}

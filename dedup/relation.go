// Package dedup builds deduplication candidate groups over a media pool and
// classifies every pair of same-duration assets as:
//
//   - RelationIdentical: the exact same media (same bytes or lossless-level
//     fingerprint match across video and audio);
//   - RelationTranscode: the same underlying shot delivered as a lossy
//     derivative (rotation / frame-rate / codec / bitrate differences, noisy
//     but clearly matching fingerprints);
//   - RelationDifferent: genuinely different shots, kept separate so a master
//     swap can never touch them.
package dedup

import (
	"encoding/json"
	"fmt"

	"mediadedup/media"
)

// Relation is the classified relationship between two assets.
type Relation int

const (
	RelationUnknown Relation = iota
	RelationIdentical
	RelationTranscode
	RelationDifferent
)

func (r Relation) String() string {
	switch r {
	case RelationIdentical:
		return "identical"
	case RelationTranscode:
		return "transcode"
	case RelationDifferent:
		return "different"
	default:
		return "unknown"
	}
}

// MarshalJSON emits the relation as a stable lowercase string.
func (r Relation) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

// UnmarshalJSON accepts the lowercase relation names.
func (r *Relation) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "identical":
		*r = RelationIdentical
	case "transcode":
		*r = RelationTranscode
	case "different":
		*r = RelationDifferent
	case "unknown", "":
		*r = RelationUnknown
	default:
		return fmt.Errorf("unknown relation %q", s)
	}
	return nil
}

// Mismatch reasons explain why a pair is a transcode or different shot.
const (
	ReasonDurationGap       = "duration outside tolerance"
	ReasonVideoMismatch     = "video fingerprints do not match"
	ReasonAudioMismatch     = "audio fingerprints do not match"
	ReasonRotationChanged   = "rotation metadata changed"
	ReasonFrameRateChanged  = "frame rate changed"
	ReasonCodecChanged      = "codec changed"
	ReasonBitrateChanged    = "bitrate changed"
	ReasonResolutionChanged = "resolution changed"
)

// Thresholds controls pair classification. All fingerprint distances are
// normalized means in [0,1].
type Thresholds struct {
	// DurationTol is the maximum duration gap (seconds) for a candidate pair.
	DurationTol float64
	// IdenticalVideo / IdenticalAudio bound the "no perceptual loss" band.
	IdenticalVideo float64
	IdenticalAudio float64
	// MatchVideo / MatchAudio bound the "same content, lossy derivative" band.
	MatchVideo float64
	MatchAudio float64
}

// DefaultThresholds returns conservative thresholds: identical requires an
// essentially perfect hash match, transcode tolerates re-encode noise, and
// anything beyond is a different shot.
func DefaultThresholds() Thresholds {
	return Thresholds{
		DurationTol:    0.25,
		IdenticalVideo: 0.02,
		IdenticalAudio: 0.02,
		MatchVideo:     0.18,
		MatchAudio:     0.22,
	}
}

// PairResult is the full verdict for one asset pair.
type PairResult struct {
	A             string   `json:"a"`
	B             string   `json:"b"`
	Relation      Relation `json:"relation"`
	VideoDistance float64  `json:"videoDistance"`
	AudioDistance float64  `json:"audioDistance"`
	DurationGap   float64  `json:"durationGap"`
	Reasons       []string `json:"reasons,omitempty"`
}

// ClassifyPair evaluates a single asset pair.
//
// Safety rule: whenever video or audio evidence fails to match, the pair is
// RelationDifferent regardless of metadata similarity, so groups can never
// merge distinct shots.
func ClassifyPair(a, b media.Asset, t Thresholds) PairResult {
	res := PairResult{
		A:           a.ID,
		B:           b.ID,
		DurationGap: media.DurationDistance(a, b),
	}

	if res.DurationGap > t.DurationTol {
		res.Relation = RelationDifferent
		res.Reasons = []string{ReasonDurationGap}
		return res
	}

	res.VideoDistance = media.VideoDistance(a.Video, b.Video)
	res.AudioDistance = media.AudioDistance(a.Audio, b.Audio)

	videoMatch := res.VideoDistance <= t.MatchVideo
	audioMatch := res.AudioDistance <= t.MatchAudio

	if !videoMatch {
		res.Relation = RelationDifferent
		res.Reasons = append(res.Reasons, ReasonVideoMismatch)
	}
	if !audioMatch {
		res.Relation = RelationDifferent
		res.Reasons = append(res.Reasons, ReasonAudioMismatch)
	}
	if res.Relation == RelationDifferent {
		return res
	}

	// Content matches. Byte-identical files are always exact duplicates.
	if media.SameFile(a, b) {
		res.Relation = RelationIdentical
		return res
	}

	lossless := res.VideoDistance <= t.IdenticalVideo &&
		res.AudioDistance <= t.IdenticalAudio

	// Record every container/metadata difference motivating the verdict.
	if a.Rotation.Normalize() != b.Rotation.Normalize() {
		res.Reasons = append(res.Reasons, ReasonRotationChanged)
	}
	if frameRateDiffers(a.FrameRate, b.FrameRate) {
		res.Reasons = append(res.Reasons, ReasonFrameRateChanged)
	}
	if a.Codec != b.Codec {
		res.Reasons = append(res.Reasons, ReasonCodecChanged)
	}
	if a.BitrateK != b.BitrateK {
		res.Reasons = append(res.Reasons, ReasonBitrateChanged)
	}
	if a.UprightWidth() != b.UprightWidth() || a.UprightHeight() != b.UprightHeight() {
		res.Reasons = append(res.Reasons, ReasonResolutionChanged)
	}

	if lossless && len(res.Reasons) == 0 {
		res.Relation = RelationIdentical
		return res
	}

	// Perfect hashes with only rotation metadata differing still describe the
	// same pixels/audio: treat as identical, but keep the reason for auditing.
	if lossless && onlyRotation(res.Reasons) {
		res.Relation = RelationIdentical
		return res
	}

	res.Relation = RelationTranscode
	if len(res.Reasons) == 0 {
		// Matches inside the lossy band but above the identical band: the
		// distance itself evidences re-encode noise.
		res.Reasons = append(res.Reasons,
			fmt.Sprintf("re-encode noise (video=%.3f audio=%.3f)",
				res.VideoDistance, res.AudioDistance))
	}
	return res
}

func frameRateDiffers(a, b float64) bool {
	if a == b {
		return false
	}
	const tol = 0.01
	d := a - b
	if d < 0 {
		d = -d
	}
	return d > tol
}

func onlyRotation(reasons []string) bool {
	if len(reasons) == 0 {
		return false
	}
	for _, r := range reasons {
		if r != ReasonRotationChanged {
			return false
		}
	}
	return true
}

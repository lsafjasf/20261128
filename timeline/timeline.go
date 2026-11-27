// Package timeline performs group-scoped master replacement: when a user
// promotes an asset to master of its dedup group, every clip referencing any
// other member of that same group is rewritten to the master. Clips belonging
// to other groups or to singleton assets are never touched.
package timeline

import (
	"fmt"

	"mediadedup/dedup"
	"mediadedup/media"
)

// Clip is one timeline segment referencing a pool asset.
//
// SourceIn/SourceOut are positions in the source asset's own timeline (seconds
// from its head); Start is the timeline position of the clip head. These three
// values are preserved across replacement because every group member depicts
// the same shot over the same duration. Only asset binding and rotation
// metadata change.
type Clip struct {
	ID        string  `json:"id"`
	TrackID   string  `json:"trackId"`
	AssetID   string  `json:"assetId"`
	Start     float64 `json:"start"`
	SourceIn  float64 `json:"sourceIn"`
	SourceOut float64 `json:"sourceOut"`
}

// Timeline is an ordered collection of clips.
type Timeline struct {
	Clips []Clip
}

// Replacement records one clip rewrite.
type Replacement struct {
	ClipID         string         `json:"clipId"`
	TrackID        string         `json:"trackId"`
	FromAssetID    string         `json:"fromAssetId"`
	ToAssetID      string         `json:"toAssetId"`
	GroupID        string         `json:"groupId"`
	RotationFix    media.Rotation `json:"rotationFix"`
	FrameRateScale float64        `json:"frameRateScale"`
}

// Report summarizes a replacement run.
type Report struct {
	GroupID      string        `json:"groupId"`
	MasterID     string        `json:"masterId"`
	Replacements []Replacement `json:"replacements,omitempty"`
	// Skipped holds clip IDs that already used the master (idempotent no-ops).
	Skipped []string `json:"skipped,omitempty"`
}

// ReplaceWithMaster rewrites clips bound to non-master members of the given
// group so they reference the group's master, and returns a per-clip report.
//
// Guarantees:
//   - clips referencing assets outside the group are not modified;
//   - running twice is a no-op the second time;
//   - clip timing (Start / SourceIn / SourceOut) is preserved;
//   - rotation compensation needed to keep the same upright picture is
//     recorded so an NLE can apply it at render time.
func ReplaceWithMaster(tl *Timeline, pool *dedup.Pool, groupID string) (*Report, error) {
	group := findGroup(pool, groupID)
	if group == nil {
		return nil, fmt.Errorf("unknown dedup group: %q", groupID)
	}
	master, ok := group.Master()
	if !ok {
		return nil, fmt.Errorf("group %s has no master set", groupID)
	}

	report := &Report{GroupID: groupID, MasterID: master.ID}

	for i := range tl.Clips {
		clip := &tl.Clips[i]
		if !group.HasMember(clip.AssetID) {
			continue // different group or singleton: leave untouched
		}
		if clip.AssetID == master.ID {
			report.Skipped = append(report.Skipped, clip.ID)
			continue
		}

		from := findAsset(pool, clip.AssetID)
		rotationFix := rotationCompensation(from.Rotation, master.Rotation)
		rateScale := 1.0
		if master.FrameRate > 0 {
			// Source frame time expressed in master-rate frames.
			rateScale = from.FrameRate / master.FrameRate
		}

		report.Replacements = append(report.Replacements, Replacement{
			ClipID:         clip.ID,
			TrackID:        clip.TrackID,
			FromAssetID:    clip.AssetID,
			ToAssetID:      master.ID,
			GroupID:        groupID,
			RotationFix:    rotationFix,
			FrameRateScale: rateScale,
		})
		clip.AssetID = master.ID
	}
	return report, nil
}

// rotationCompensation returns the extra rotation to apply to the master so
// the replaced clip shows the same upright picture as the old asset.
// Both inputs are normalized storage rotations; the result is in {0,90,180,270}.
func rotationCompensation(from, to media.Rotation) media.Rotation {
	delta := (int(from.Normalize()) - int(to.Normalize()) + 360) % 360
	return media.Rotation(delta)
}

func findGroup(pool *dedup.Pool, groupID string) *dedup.Group {
	for _, g := range pool.Groups {
		if g.ID == groupID {
			return g
		}
	}
	return nil
}

func findAsset(pool *dedup.Pool, id string) media.Asset {
	for _, a := range pool.Assets {
		if a.ID == id {
			return a
		}
	}
	return media.Asset{}
}

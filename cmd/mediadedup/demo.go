package main

import (
	"fmt"
	"io"

	"mediadedup/dedup"
	"mediadedup/media"
	"mediadedup/timeline"
)

const (
	shotAHash uint64 = 0x0F0F0F0F0F0F0F0F
	shotBHash uint64 = 0xF0F0F0F0F0F0F0F0
	shotCHash uint64 = 0xAAAAAAAAAAAAAAAA
)

// demoAssets reproduces the scenario from the brief: same-named media with
// equal duration but different rotation, frame rate and audio fingerprints.
func demoAssets() []media.Asset {
	return []media.Asset{
		// Shot "interview": three same-duration representations.
		demoAsset("interview_01.mov", "interview", 12.0, 3840, 2160, 60, media.Rotate0, "prores", 48000, shotAHash, shotAHash),
		demoAsset("interview_02.mov", "interview", 12.0, 2160, 3840, 24, media.Rotate90, "h264", 9000, shotAHash, shotAHash),
		demoNoisy("interview_03.mov", "interview", 12.04, 1920, 1080, 30, media.Rotate0, "h265", 12000, shotAHash, shotAHash, 3),
		// Shot "broll": two representations, distinct picture + audio.
		demoAsset("broll_01.mov", "broll", 12.0, 1920, 1080, 30, media.Rotate0, "prores", 40000, shotBHash, shotBHash),
		demoNoisy("broll_02.mov", "broll", 12.0, 1080, 1920, 30, media.Rotate270, "h264", 7000, shotBHash, shotBHash, 2),
		// A real different shot that merely shares the duration and name stem.
		demoAsset("interview_extra.mov", "interview", 12.0, 1920, 1080, 30, media.Rotate0, "h264", 6000, shotCHash, shotCHash),
		// Singleton with a different duration.
		demoAsset("ending.mov", "ending", 30.0, 1920, 1080, 30, media.Rotate0, "h264", 5000, 0x5555555555555555, 0x5555555555555555),
	}
}

func demoAsset(path, stem string, dur float64, w, h, fps int, rot media.Rotation, codec string, bitrate int, vh, ah uint64) media.Asset {
	const samples = 12
	return media.Asset{
		ID: stem + "#" + path, Path: path, Duration: dur,
		Width: w, Height: h, FrameRate: float64(fps), Rotation: rot,
		Codec: codec, BitrateK: bitrate,
		Video: media.VideoFingerprint{Rotation: rot, Frames: hashLadder(vh, samples)},
		Audio: media.AudioFingerprint{Points: hashLadder(ah, samples)},
	}
}

func demoNoisy(path, stem string, dur float64, w, h, fps int, rot media.Rotation, codec string, bitrate int, vh, ah uint64, noise uint) media.Asset {
	a := demoAsset(path, stem, dur, w, h, fps, rot, codec, bitrate, vh, ah)
	for i := range a.Video.Frames {
		a.Video.Frames[i] ^= (uint64(1) << noise) - 1
	}
	for i := range a.Audio.Points {
		a.Audio.Points[i] ^= (uint64(1) << noise) - 1
	}
	return a
}

func hashLadder(base uint64, n int) []uint64 {
	out := make([]uint64, n)
	for i := range out {
		out[i] = base + uint64(i)*0x1111111111111111
	}
	return out
}

func demoTimeline() *timeline.Timeline {
	return &timeline.Timeline{Clips: []timeline.Clip{
		{ID: "clip-1", TrackID: "V1", AssetID: "interview#interview_02.mov", Start: 0, SourceIn: 0, SourceOut: 6},
		{ID: "clip-2", TrackID: "V1", AssetID: "interview#interview_03.mov", Start: 6, SourceIn: 0, SourceOut: 6},
		{ID: "clip-3", TrackID: "V1", AssetID: "broll#broll_02.mov", Start: 12, SourceIn: 0, SourceOut: 4},
		{ID: "clip-4", TrackID: "V2", AssetID: "ending#ending.mov", Start: 16, SourceIn: 2, SourceOut: 10},
	}}
}

func runDemo(out io.Writer) error {
	pool := dedup.Build(demoAssets(), dedup.DefaultThresholds())

	fmt.Fprintln(out, "== dedup candidate groups ==")
	for _, g := range pool.Groups {
		fmt.Fprintf(out, "\n%s  master=%s\n", g.ID, g.MasterID)
		for _, a := range g.Members {
			fmt.Fprintf(out, "  - %-22s %5.2fs %4dx%-4d %2dfps rot=%-3d %-6s %5dk\n",
				a.Path, a.Duration, a.UprightWidth(), a.UprightHeight(),
				int(a.FrameRate), a.Rotation, a.Codec, a.BitrateK)
		}
		for key, r := range g.Relations {
			fmt.Fprintf(out, "    pair %-40s %-9s v=%.3f a=%.3f %v\n",
				key, r.Relation, r.VideoDistance, r.AudioDistance, r.Reasons)
		}
	}
	fmt.Fprintln(out, "\nsingletons (never auto-replaced):")
	for _, a := range pool.Assets {
		if _, ok := pool.GroupOf(a.ID); !ok {
			fmt.Fprintf(out, "  - %s\n", a.Path)
		}
	}

	group, ok := pool.GroupOf("interview#interview_01.mov")
	if !ok {
		return fmt.Errorf("interview group missing")
	}
	fmt.Fprintf(out, "\n== user promotes master: %s ==\n", group.MasterID)

	tl := demoTimeline()
	report, err := timeline.ReplaceWithMaster(tl, pool, group.ID)
	if err != nil {
		return err
	}
	for _, r := range report.Replacements {
		fmt.Fprintf(out, "replaced %s: %s -> %s (rotationFix=%d, rateScale=%.3f)\n",
			r.ClipID, r.FromAssetID, r.ToAssetID, r.RotationFix, r.FrameRateScale)
	}
	for _, id := range report.Skipped {
		fmt.Fprintf(out, "skipped  %s: already on master\n", id)
	}
	fmt.Fprintln(out, "\nuntouched clips (other groups / singletons):")
	for _, c := range tl.Clips {
		if !group.HasMember(c.AssetID) {
			fmt.Fprintf(out, "  - %s -> %s\n", c.ID, c.AssetID)
		}
	}

	// Second run proves idempotency.
	again, err := timeline.ReplaceWithMaster(tl, pool, group.ID)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\nsecond run: %d replacements, %d skipped (idempotent)\n",
		len(again.Replacements), len(again.Skipped))
	return nil
}

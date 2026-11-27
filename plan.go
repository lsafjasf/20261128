package subburn

import (
	"fmt"
	"time"
)

type BurnPlan struct {
	Rate     Rate
	Preroll  time.Duration
	Duration time.Duration
	Cues     []CuePlacement
}

type CuePlacement struct {
	CueID        string
	FirstFrame   int64
	EndFrame     int64
	DisplayStart time.Duration
	DisplayEnd   time.Duration
	EncodedStart time.Duration
	EncodedEnd   time.Duration
}

func (p CuePlacement) FrameCount() int64 {
	return p.EndFrame - p.FirstFrame
}

// Exact returns a renderer clock with the same real rate and preroll but none
// of the timestamp defects. Preroll remains in Encoded PTS and is explicitly
// excluded from subtitle lookup.
func (c RenderClock) Exact() RenderClock {
	return RenderClock{
		Rate:    c.Rate,
		Preroll: c.Preroll,
	}
}

func (p Pipeline) Corrected() Pipeline {
	return Pipeline{
		Duration: p.Duration,
		Preview:  p.Preview.Exact(),
		Export:   p.Export.Exact(),
	}
}

// BuildBurnPlan maps subtitles on the source display clock. Frame selection
// uses [Start, End) against exact frame PTS; it never scans encoded PTS.
func BuildBurnPlan(rate Rate, preroll, duration time.Duration, cues []Cue) (BurnPlan, error) {
	if !rate.Valid() {
		return BurnPlan{}, fmt.Errorf("invalid rate: %s", rate)
	}
	if preroll < 0 {
		return BurnPlan{}, fmt.Errorf("preroll must not be negative")
	}
	if duration <= 0 {
		return BurnPlan{}, fmt.Errorf("duration must be positive")
	}
	placements := make([]CuePlacement, 0, len(cues))
	frameCount := rate.FrameCount(duration)
	for _, cue := range cues {
		if err := cue.Validate(); err != nil {
			return BurnPlan{}, err
		}
		if cue.End > duration {
			return BurnPlan{}, fmt.Errorf("cue %q ends after media duration", cue.ID)
		}
		first := rate.FrameAtOrAfter(cue.Start)
		end := rate.FrameAtOrAfter(cue.End)
		if first >= frameCount {
			return BurnPlan{}, fmt.Errorf("cue %q starts after the last displayed frame", cue.ID)
		}
		if end <= first {
			return BurnPlan{}, fmt.Errorf("cue %q is shorter than one frame", cue.ID)
		}
		displayStart := rate.FramePTS(first)
		displayEnd := rate.FramePTS(end)
		placements = append(placements, CuePlacement{
			CueID:        cue.ID,
			FirstFrame:   first,
			EndFrame:     end,
			DisplayStart: displayStart,
			DisplayEnd:   displayEnd,
			EncodedStart: displayStart + preroll,
			EncodedEnd:   displayEnd + preroll,
		})
	}
	return BurnPlan{
		Rate:     rate,
		Preroll:  preroll,
		Duration: duration,
		Cues:     placements,
	}, nil
}

// SubtitleQueryTime is the time a renderer must use for a frame after decoding
// preroll. It is encoded PTS minus preroll, which is the display PTS.
func (p BurnPlan) SubtitleQueryTime(encodedPTS time.Duration) time.Duration {
	return encodedPTS - p.Preroll
}

// Verify compares the corrected plan with the preview's display onsets.
func (p BurnPlan) Verify(preview RenderClock, cues []Cue) error {
	if len(cues) != len(p.Cues) {
		return fmt.Errorf("plan has %d placements for %d cues", len(p.Cues), len(cues))
	}
	seen := make(map[string]CuePlacement, len(p.Cues))
	for _, placement := range p.Cues {
		seen[placement.CueID] = placement
	}
	maxError := maxDuration(preview.Rate.FrameDuration(), p.Rate.FrameDuration())
	for _, cue := range cues {
		placement, ok := seen[cue.ID]
		if !ok {
			return fmt.Errorf("cue %q has no placement", cue.ID)
		}
		previewFrame, ok := preview.OnsetFrame(cue.Start, p.Duration)
		if !ok {
			return fmt.Errorf("cue %q has no preview onset", cue.ID)
		}
		previewDisplay := preview.TrueTime(previewFrame)
		if absDuration(placement.DisplayStart-previewDisplay) > maxError {
			return fmt.Errorf("cue %q starts %s away from preview",
				cue.ID, placement.DisplayStart-previewDisplay)
		}
		query := p.SubtitleQueryTime(placement.EncodedStart)
		if query != placement.DisplayStart {
			return fmt.Errorf("cue %q subtitle query clock drifted by %s",
				cue.ID, query-placement.DisplayStart)
		}
	}
	return nil
}

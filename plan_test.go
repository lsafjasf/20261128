package subburn

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestCorrectedPlanUsesSourceCueStartsWithoutFixedOffset(t *testing.T) {
	rate := mustRate(t, "30000/1001")
	cues := standardCues()
	broken := Pipeline{
		Duration: 300 * time.Second,
		Preview:  ExactClock(rate),
		Export: RenderClock{
			Rate:      rate,
			FrameStep: 34 * time.Millisecond,
			Preroll:   3 * time.Second,
		},
	}
	before, err := broken.CompareCues(cues)
	if err != nil {
		t.Fatalf("CompareCues: %v", err)
	}
	if before[len(before)-1].Error > -time.Second {
		t.Fatalf("test setup did not reproduce cumulative drift: %s", before[len(before)-1].Error)
	}

	corrected := broken.Corrected()
	plan, err := BuildBurnPlan(corrected.Export.Rate, corrected.Export.Preroll,
		corrected.Duration, cues)
	if err != nil {
		t.Fatalf("BuildBurnPlan: %v", err)
	}
	if err := plan.Verify(corrected.Preview, cues); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if plan.Preroll != 3*time.Second {
		t.Fatalf("preroll = %s, want 3s", plan.Preroll)
	}
	for i, placement := range plan.Cues {
		if placement.EncodedStart-placement.DisplayStart != plan.Preroll {
			t.Fatalf("cue %d encoded PTS mismatch", i)
		}
		if got := plan.SubtitleQueryTime(placement.EncodedStart); got != placement.DisplayStart {
			t.Fatalf("cue %d query time = %s, display = %s", i, got, placement.DisplayStart)
		}
	}

	correctedComparisons, err := corrected.CompareCues(cues)
	if err != nil {
		t.Fatalf("corrected CompareCues: %v", err)
	}
	maxError := corrected.Preview.Rate.FrameDuration()
	for _, comparison := range correctedComparisons {
		if absDuration(comparison.Error) > maxError {
			t.Fatalf("cue %s still differs by %s", comparison.CueID, comparison.Error)
		}
	}
}

func TestSRTRoundTripPreservesCueTimes(t *testing.T) {
	input := "\ufeff1\r\n00:00:01,250 --> 00:00:03,500\r\nfirst line\r\nsecond line\r\n\r\n" +
		"2\r\n01:02:03,004 --> 01:02:04,005\r\nlater\r\n"
	cues, err := ParseSRT(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(cues) != 2 || cues[0].Start != 1250*time.Millisecond ||
		cues[0].End != 3500*time.Millisecond || cues[1].Start != time.Hour+2*time.Minute+3*time.Second+4*time.Millisecond {
		t.Fatalf("unexpected cues: %+v", cues)
	}
	var output bytes.Buffer
	if err := FormatSRT(&output, cues); err != nil {
		t.Fatalf("FormatSRT: %v", err)
	}
	roundTrip, err := ParseSRT(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatalf("round-trip ParseSRT: %v", err)
	}
	for i := range cues {
		if cues[i].Start != roundTrip[i].Start || cues[i].End != roundTrip[i].End {
			t.Fatalf("cue %d time changed: %+v -> %+v", i, cues[i], roundTrip[i])
		}
	}
}

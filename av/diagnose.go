package av

import (
	"fmt"
	"math"
)

// Cause identifies the root cause of per-cut audio blanks.
type Cause int

const (
	CauseNone        Cause = iota // no defect found
	CauseReorderWait              // audio held back during B-frame reorder flush
	CauseEditList                 // bad elst shifts video presentation vs audio
	CauseClockReset               // video clock restarted at each shot
)

func (c Cause) String() string {
	switch c {
	case CauseReorderWait:
		return "reorder-wait (重排等待)"
	case CauseEditList:
		return "edit-list (编辑列表)"
	case CauseClockReset:
		return "clock-reset (时钟重置)"
	default:
		return "none"
	}
}

// Diagnosis is the result of classifying the event table.
type Diagnosis struct {
	Cause          Cause
	ReorderDelayMs float64
	Evidence       []string
	Cuts           []CutInfo
}

// toleranceMs is the slack for matching discontinuities to the reorder delay.
const toleranceMs = 8.0

// Diagnose classifies the root cause from the event table.
//
// Decision order matters: a hole in the audio media timeline itself can only
// come from the capture/encode side (reorder wait); a hole that exists only
// in the video media timeline is a clock reset; if both media timelines are
// continuous, the misalignment must come from the edit list.
func Diagnose(m *Model, t *EventTable) *Diagnosis {
	d := &Diagnosis{
		Cuts:           t.Cuts,
		ReorderDelayMs: m.Video.Ms(m.ReorderDelay()),
	}
	if len(t.Cuts) == 0 {
		d.Evidence = append(d.Evidence, "no scene cuts (single sync sample); nothing to classify")
		return d
	}
	var videoJumpSizes, elstShifts []float64
	for _, c := range t.Cuts {
		videoJumpSizes = append(videoJumpSizes, c.VideoDTSJumpMs)
		elstShifts = append(elstShifts, c.ElstShiftMs)
	}
	medVideo := median(videoJumpSizes)
	medElst := medianAbs(elstShifts)

	// Associate audio discontinuities with cuts in order. A reorder-wait
	// hole drifts late by the accumulated per-cut delay, so the i-th jump
	// may sit up to i*reorderDelay after its cut.
	audioMatch := len(t.AudioJumps) == len(t.Cuts)
	var audioJumpSizes []float64
	audioFrameMs := m.Audio.Ms(m.AudioFrameDur())
	for i, j := range t.AudioJumps {
		audioJumpSizes = append(audioJumpSizes, j.JumpMs)
		if !audioMatch {
			continue
		}
		lo := t.Cuts[i].CutPresMs - toleranceMs
		hi := t.Cuts[i].CutPresMs + float64(i+1)*d.ReorderDelayMs + audioFrameMs + toleranceMs
		if j.AtMs < lo || j.AtMs > hi {
			audioMatch = false
		}
		if j.JumpMs < toleranceMs || math.Abs(j.JumpMs-d.ReorderDelayMs) > toleranceMs {
			audioMatch = false
		}
	}
	medAudio := median(audioJumpSizes)
	d.Evidence = append(d.Evidence,
		fmt.Sprintf("video reorder delay (max CTS-DTS): %.1f ms", d.ReorderDelayMs),
		fmt.Sprintf("audio timeline discontinuities: %d (median %.1f ms), cuts: %d", len(t.AudioJumps), medAudio, len(t.Cuts)),
		fmt.Sprintf("median video DTS excess jump at cuts: %.1f ms", medVideo),
		fmt.Sprintf("median |elst shift| at cuts: %.1f ms", medElst),
	)
	switch {
	case audioMatch && len(t.AudioJumps) > 0:
		d.Cause = CauseReorderWait
		d.Evidence = append(d.Evidence,
			fmt.Sprintf("audio media timeline itself has %d holes of ~%.1f ms, one per cut, matching the %.1f ms reorder delay: audio frames were held while the encoder flushed its B-frame reorder buffer", len(t.AudioJumps), medAudio, d.ReorderDelayMs),
			"video stts/ctts tables are clean, so the defect is on the audio capture path, not in mapping metadata",
		)
	case medVideo > toleranceMs:
		d.Cause = CauseClockReset
		d.Evidence = append(d.Evidence,
			fmt.Sprintf("video DTS jumps forward by %.1f ms beyond one frame at every cut while the audio timeline stays continuous", medVideo),
			fmt.Sprintf("skew grows cumulatively: %.1f ms after the last cut (per-shot clock restart)", medVideo*float64(len(t.Cuts))),
		)
	case medElst > toleranceMs:
		d.Cause = CauseEditList
		d.Evidence = append(d.Evidence,
			fmt.Sprintf("both media timelines are continuous, but the video edit list shifts presentation by %.1f ms at the cuts", medElst),
			"players align audio to the video movie-time clock, so the shift surfaces as an audio blank at each cut",
		)
	default:
		d.Cause = CauseNone
		d.Evidence = append(d.Evidence, "audio coverage is continuous at every cut; no defect detected")
	}
	return d
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64{}, v...)
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s[len(s)/2]
}

func medianAbs(v []float64) float64 {
	abs := make([]float64, len(v))
	for i, x := range v {
		abs[i] = math.Abs(x)
	}
	return median(abs)
}

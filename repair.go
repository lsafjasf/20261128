package main

import "math"

// RepairMode selects what gets removed.
type RepairMode string

const (
	ModeFixed    RepairMode = "fixed"    // subtract constant DC only
	ModeDrift    RepairMode = "drift"    // subtract interpolated trend (fixed+drift)
	ModeSubsonic RepairMode = "subsonic" // also remove true ultra-low-freq
)

type SegmentDiff struct {
	Index      int     `json:"index"`
	StartSec   float64 `json:"start_sec"`
	Changed    int     `json:"changed_samples"`
	Total      int     `json:"total_samples"`
	MaxAbsDiff float64 `json:"max_abs_diff"`
	RMSDiff    float64 `json:"rms_diff"`
	MeanDiff   float64 `json:"mean_diff"`
}

type ChannelRepair struct {
	Channel  int           `json:"channel"`
	Mode     RepairMode    `json:"mode"`
	Segments []SegmentDiff `json:"segments"`
}

// RepairChannel removes DC/drift from x in place and returns per-segment
// diffs against the source samples. The trend is built from per-segment
// means, linearly interpolated between segment centers to avoid steps.
func RepairChannel(ch int, x []float64, sampleRate int, mode RepairMode, keepSubsonic bool) ChannelRepair {
	r := ChannelRepair{Channel: ch, Mode: mode}
	n := len(x)
	if n == 0 {
		return r
	}
	src := append([]float64(nil), x...)

	segLen := int(SegmentLen * float64(sampleRate))
	if segLen < 64 {
		segLen = 64
	}
	nSeg := (n + segLen - 1) / segLen
	means := make([]float64, nSeg)
	for s := 0; s < nSeg; s++ {
		lo, hi := s*segLen, min((s+1)*segLen, n)
		var sum float64
		for i := lo; i < hi; i++ {
			sum += src[i]
		}
		means[s] = sum / float64(hi-lo)
	}

	corr := make([]float64, n)
	for i := 0; i < n; i++ {
		pos := float64(i)/float64(segLen) - 0.5
		switch mode {
		case ModeFixed:
			var sum float64
			for _, m := range means {
				sum += m
			}
			corr[i] = sum / float64(nSeg)
		default:
			k := int(math.Floor(pos))
			frac := pos - float64(k)
			if k < 0 {
				corr[i] = means[0]
			} else if k >= nSeg-1 {
				corr[i] = means[nSeg-1]
			} else {
				corr[i] = means[k]*(1-frac) + means[k+1]*frac
			}
		}
	}

	// keepSubsonic: only remove the ultra-slow part of the correction so
	// genuine <20Hz program material survives.
	if keepSubsonic && mode != ModeFixed {
		win := sampleRate / 20
		if win > 1 && n > win {
			slow := movingAvg(corr, win)
			copy(corr, slow)
		}
	}

	for i := range x {
		x[i] = src[i] - corr[i]
	}

	for s := 0; s < nSeg; s++ {
		lo, hi := s*segLen, min((s+1)*segLen, n)
		d := SegmentDiff{Index: s, StartSec: float64(lo) / float64(sampleRate), Total: hi - lo}
		var sq, sum float64
		for i := lo; i < hi; i++ {
			diff := src[i] - x[i]
			if diff != 0 {
				d.Changed++
			}
			if math.Abs(diff) > d.MaxAbsDiff {
				d.MaxAbsDiff = math.Abs(diff)
			}
			sq += diff * diff
			sum += diff
		}
		d.RMSDiff = math.Sqrt(sq / float64(hi-lo))
		d.MeanDiff = sum / float64(hi-lo)
		r.Segments = append(r.Segments, d)
	}
	return r
}

func movingAvg(x []float64, win int) []float64 {
	out := make([]float64, len(x))
	var run float64
	for i := range x {
		run += x[i]
		if i >= win {
			run -= x[i-win]
		}
		lo := 0
		if i-win+1 > 0 {
			lo = i - win + 1
		}
		out[i] = run / float64(i-lo+1)
	}
	return out
}

package analysis

import (
	"math"
	"sort"

	"resampleforensics/internal/dsp"
)

func boundaryReport(source []float64, srcRate, dstRate int, dst []float64, lag int, ref []float64, opts Options, bandwidth float64, targetHalf int) BoundaryReport {
	window := int(opts.BoundaryWindowMs / 1000 * float64(dstRate))
	if window < dstRate/50 {
		window = dstRate / 50
	}
	interior := interiorNRMSE(ref, dst, dstRate, lag)
	if bandwidth <= 0 || bandwidth >= math.Min(float64(srcRate), float64(dstRate))/2 {
		bandwidth = opts.CutoffHz
	}
	halfWidths := candidateHalfWidths(targetHalf)
	var scores []BoundaryScore
	for _, half := range halfWidths {
		rs, err := dsp.NewSincResampler(srcRate, dstRate, bandwidth, half)
		if err != nil {
			continue
		}
		nOut := rs.OutputLength(len(source))
		for _, mode := range []dsp.BoundaryMode{dsp.BoundaryZero, dsp.BoundaryReflect, dsp.BoundaryClamp} {
			start, end := edgeScores(rs, source, dst, dstRate, lag, nOut, mode, window, interior)
			if math.IsNaN(start) && math.IsNaN(end) {
				continue
			}
			combined := math.NaN()
			if !math.IsNaN(start) && !math.IsNaN(end) {
				combined = math.Sqrt((start*start + end*end) / 2)
			} else if !math.IsNaN(start) {
				combined = start
			} else {
				combined = end
			}
			scores = append(scores, BoundaryScore{
				Mode: mode.String(), HalfWidth: half,
				Start: start, End: end, Combined: combined,
			})
		}
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].Combined < scores[j].Combined })
	best := BoundaryScore{Combined: math.Inf(1)}
	runner := math.Inf(1)
	if len(scores) > 0 {
		best = scores[0]
		for _, score := range scores[1:] {
			if score.Mode != best.Mode && score.Combined < runner {
				runner = score.Combined
			}
		}
	}
	margin := 0.0
	if runner > 1e-12 && !math.IsInf(runner, 1) {
		margin = (runner - best.Combined) / runner
	}
	sameModeNearTop := 0
	for i := 0; i < len(scores) && i < 5; i++ {
		if scores[i].Mode == best.Mode {
			sameModeNearTop++
		}
	}
	consistent := sameModeNearTop >= 3
	leadingExtra := 0.0
	if lag > 0 {
		leadingExtra = float64(lag) / float64(dstRate) * 1000
	}
	trailingExtra := 0.0
	if extra := len(dst) - lag - len(ref); extra > 0 {
		trailingExtra = float64(extra) / float64(dstRate) * 1000
	}
	return BoundaryReport{
		WindowMS: float64(window) / float64(dstRate) * 1000,
		Scores:   scores, BestMode: best.Mode, BestHalfWidth: best.HalfWidth,
		BestMargin: margin, Indeterminate: len(scores) < 3 || (!consistent && margin < 0.05),
		LeadingExtraMS: leadingExtra, TrailingExtraMS: trailingExtra,
		InteriorNRMSE: interior,
	}
}

func candidateHalfWidths(target int) []int {
	set := map[int]bool{4: true, 6: true, 8: true, 12: true, 16: true, 24: true, 32: true, 48: true, 64: true}
	if target > 0 {
		for _, v := range []int{target / 2, target, target * 2} {
			if v >= 4 && v <= 128 {
				set[v] = true
			}
		}
	}
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

func edgeScores(rs *dsp.SincResampler, source, dst []float64, rate, lag, nOut int, mode dsp.BoundaryMode, window int, globalScale float64) (float64, float64) {
	startIndex := 0
	if lag < 0 {
		startIndex = -lag
	}
	start := math.NaN()
	if startIndex+window+lag <= len(dst) && startIndex+window <= nOut {
		r := rs.ResampleSegment(source, mode, startIndex, window)
		d := dst[startIndex+lag : startIndex+window+lag]
		start = shortTimeEdgeError(r, d, rate, globalScale)
	}
	endIndex := nOut
	if len(dst)-lag < endIndex {
		endIndex = len(dst) - lag
	}
	end := math.NaN()
	if endIndex-window >= 0 {
		r := rs.ResampleSegment(source, mode, endIndex-window, window)
		d := dst[endIndex-window+lag : endIndex+lag]
		end = shortTimeEdgeError(r, d, rate, globalScale)
	}
	return start, end
}

func shortTimeEdgeError(ref, dst []float64, rate int, globalScale float64) float64 {
	if len(ref) == 0 || len(ref) != len(dst) {
		return math.NaN()
	}
	sub := int(0.002 * float64(rate))
	if sub < 8 {
		sub = 8
	}
	if sub > len(ref) {
		sub = len(ref)
	}
	worst := 0.0
	for lo := 0; lo+sub <= len(ref); lo += sub {
		r := ref[lo : lo+sub]
		d := dst[lo : lo+sub]
		local := math.Max(dsp.RMS(r), dsp.RMS(d))
		if globalScale > 0 {
			local = math.Max(local, globalScale*0.01)
		}
		e := relativeError(r, d, local)
		if e > worst {
			worst = e
		}
	}
	return worst
}

func interiorNRMSE(ref, dst []float64, rate, lag int) float64 {
	rStart := int(0.15 * float64(rate))
	if rStart+lag < 0 {
		rStart = -lag
	}
	dStart := rStart + lag
	hi := len(ref) - int(0.15*float64(rate))
	if len(dst)-lag < hi {
		hi = len(dst) - lag
	}
	count := hi - rStart
	if count > len(dst)-dStart {
		count = len(dst) - dStart
	}
	if count <= 0 {
		return math.NaN()
	}
	return relativeError(ref[rStart:rStart+count], dst[dStart:dStart+count], dsp.RMS(ref[rStart:rStart+count]))
}

func relativeError(a, b []float64, scale float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return math.NaN()
	}
	if scale <= 1e-12 {
		return math.Inf(1)
	}
	var sum float64
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return math.Sqrt(sum/float64(len(a))) / scale
}

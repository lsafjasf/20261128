package audit

import (
	"fmt"
	"math"
	"timesync/audio"
	"timesync/remap"
	"timesync/subs"
)

const (
	timelineTol = 2e-3 // seconds
	pitchTol    = 0.05 // 5% relative
	sampleTol   = 2    // frames of rounding slack
)

// Inputs bundles everything the audit inspects. All three layers are
// cross-checked against the same curve independently.
type Inputs struct {
	Plan         remap.SamplePlan
	Source       []float64
	LinearOutput []float64
	TSMOutput    []float64
	SourceEvents []subs.Event
	MappedEvents []subs.MappedEvent
}

// Run performs the full segment-by-segment audit.
func Run(in Inputs) *Report {
	r := &Report{Pass: true}
	curve := in.Plan.Curve
	segs := curve.Segments()

	r.Segments = make([]SegmentReport, 0, len(segs))
	for i, seg := range segs {
		r.Segments = append(r.Segments, auditSegment(in, i, seg))
	}

	r.Offset = rejectGlobalOffset(curve)
	if !r.Offset.Rejected {
		r.Pass = false
		r.add(SevError, "offset", r.Offset.Message)
	}

	auditPitch(in, r)
	auditSubtitles(in, r)
	auditFrameCounts(in, r)

	for _, f := range r.Findings {
		if f.Severity == SevError {
			r.Pass = false
		}
	}
	return r
}

func auditSegment(in Inputs, idx int, seg remap.Segment) SegmentReport {
	plan := in.Plan
	t0 := plan.Curve.Map(seg.S0)
	t1 := plan.Curve.Map(seg.S1)
	rep := SegmentReport{
		Index:              idx,
		SourceStart:        seg.S0,
		SourceEnd:          seg.S1,
		OutputStart:        t0,
		OutputEnd:          t1,
		RateStart:          seg.RateStart,
		RateEnd:            seg.RateEnd,
		SourceSamples:      int(math.Round((seg.S1 - seg.S0) * float64(plan.SourceRateHz))),
		ExpectedOutSamples: int(math.Round((t1 - t0) * float64(plan.OutputRateHz))),
	}

	first := plan.OutputFrameAt(t0)
	last := plan.OutputFrameAt(t1)
	rep.ObservedOutSamples = last - first
	rep.LocalRateCheck = true
	if math.Abs(float64(rep.ObservedOutSamples-rep.ExpectedOutSamples)) > sampleTol {
		rep.LocalRateCheck = false
	}

	// Round-trip several probes inside this segment only.
	for _, frac := range []float64{0.1, 0.5, 0.9} {
		s := seg.S0 + frac*(seg.S1-seg.S0)
		back := plan.Curve.Inverse(plan.Curve.Map(s))
		if err := math.Abs(back - s); err > rep.MapInverseMaxErr {
			rep.MapInverseMaxErr = err
		}
	}

	// Local slope dt/ds must equal 1/rate at both ends of the segment.
	h := (seg.S1 - seg.S0) / 1000
	slope0 := (plan.Curve.Map(seg.S0+h) - plan.Curve.Map(seg.S0)) / h
	slope1 := (plan.Curve.Map(seg.S1) - plan.Curve.Map(seg.S1-h)) / h
	want0 := 1 / seg.RateStart
	want1 := 1 / seg.RateEnd
	rep.LocalRateCheck = rep.LocalRateCheck &&
		math.Abs(slope0-want0) <= pitchTol*want0 &&
		math.Abs(slope1-want1) <= pitchTol*want1 &&
		rep.MapInverseMaxErr < timelineTol
	return rep
}

// rejectGlobalOffset fits the best single affine line t = alpha*s + delta
// against all segment-boundary anchors. If one global offset could patch
// the curve, every anchor would lie on this line. Real piecewise curves
// leave residuals far above the timeline tolerance.
func rejectGlobalOffset(curve *remap.Curve) OffsetCheck {
	var anchors [][2]float64
	for i, seg := range curve.Segments() {
		if i == 0 {
			anchors = append(anchors, [2]float64{seg.S0, curve.Map(seg.S0)})
		}
		anchors = append(anchors, [2]float64{seg.S1, curve.Map(seg.S1)})
	}
	n := float64(len(anchors))
	var sx, st, sxx, sxt float64
	for _, a := range anchors {
		sx += a[0]
		st += a[1]
		sxx += a[0] * a[0]
		sxt += a[0] * a[1]
	}
	alpha := (n*sxt - sx*st) / (n*sxx - sx*sx)
	delta := (st - alpha*sx) / n
	maxRes := 0.0
	for _, a := range anchors {
		if e := math.Abs(a[1] - (alpha*a[0] + delta)); e > maxRes {
			maxRes = e
		}
	}
	rejected := maxRes > timelineTol
	msg := fmt.Sprintf(
		"best global-offset fit t=%.6f*s%+.6f leaves max residual %.3fms (tol %.0fms)",
		alpha, delta, maxRes*1000, timelineTol*1000)
	if !rejected {
		msg = "global-offset patch unexpectedly fits within tolerance: " + msg
	}
	return OffsetCheck{Rejected: rejected, BestOffset: delta, Residual: maxRes, Message: msg}
}

func auditPitch(in Inputs, r *Report) {
	srcRate := in.Plan.SourceRateHz
	outRate := in.Plan.OutputRateHz
	srcPitch, _ := audio.EstimatePitchHz(in.Source, srcRate)
	linPitch, _ := audio.EstimatePitchHz(in.LinearOutput, outRate)
	tsmPitch, _ := audio.EstimatePitchHz(in.TSMOutput, outRate)

	r.Pitch = PitchCheck{
		SourcePitchHz:       srcPitch,
		LinearOutputPitchHz: linPitch,
		TSMOutputPitchHz:    tsmPitch,
		LinearDriftRatio:    linPitch / srcPitch,
		TSMDriftRatio:       tsmPitch / srcPitch,
		TSMPitchPreserved:   math.Abs(tsmPitch/srcPitch-1) <= pitchTol,
	}
	if !r.Pitch.TSMPitchPreserved {
		r.Pass = false
		r.add(SevError, "pitch",
			fmt.Sprintf("TSM output pitch drift %.2f%% exceeds %.0f%%",
				(r.Pitch.TSMDriftRatio-1)*100, pitchTol*100))
	}
}

func auditSubtitles(in Inputs, r *Report) {
	ok := subs.OrderingIntact(in.MappedEvents) &&
		subs.TextPreserved(in.SourceEvents, in.MappedEvents)
	// Re-derive each event from the curve: any event carried by a shared
	// offset instead of its own segment would diverge here.
	for i, ev := range in.MappedEvents {
		if math.Abs(in.Plan.Curve.Map(ev.Start)-ev.StartOut) > timelineTol ||
			math.Abs(in.Plan.Curve.Map(ev.End)-ev.EndOut) > timelineTol {
			ok = false
			r.add(SevError, fmt.Sprintf("subtitle[%d]", i),
				"event timing does not match independent curve mapping")
		}
	}
	r.SubtitleOK = ok
	if !ok {
		r.SubtitleDetail = "ordering, text, or independent mapping check failed"
		r.Pass = false
	} else {
		r.SubtitleDetail = fmt.Sprintf("%d events: text preserved, order intact, per-event mapping verified",
			len(in.MappedEvents))
	}
}

func auditFrameCounts(in Inputs, r *Report) {
	want := in.Plan.OutputFrameCount()
	for name, buf := range map[string][]float64{
		"linear": in.LinearOutput,
		"tsm":    in.TSMOutput,
	} {
		if math.Abs(float64(len(buf)-want)) > sampleTol {
			r.Pass = false
			r.add(SevError, "frames:"+name,
				fmt.Sprintf("output has %d frames, curve requires %d", len(buf), want))
		}
	}
	for i := range r.Segments {
		if !r.Segments[i].LocalRateCheck {
			r.Pass = false
			r.add(SevError, fmt.Sprintf("segment[%d]", i),
				"local rate/sample-count/round-trip check failed")
		}
	}
}

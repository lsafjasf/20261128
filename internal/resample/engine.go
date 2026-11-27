package resample

import "math"

const (
	KindExact       = "exact"
	KindRepeat      = "repeat"
	KindSynthesized = "synthesized"
	KindGapHold     = "gap_hold"
	KindTailHold    = "tail_hold"
)

const (
	ClassBootstrap           = "bootstrap"
	ClassSmooth              = "smooth"
	ClassRepeatPause         = "repeat_pause"
	ClassRepeatJump          = "repeat_jump"
	ClassMissingSourcePause  = "missing_source_pause"
	ClassMissingSourceJump   = "missing_source_jump"
	ClassInterpolationJudder = "interpolation_judder"
	ClassSourceEndHold       = "source_end_hold"
)

// Config is one full analysis run for one strategy.
type Config struct {
	SourceRate   Rate
	OutputRate   Rate
	SourceFrames int
	Missing      map[int]bool // nominal source indices that are absent
	Strategy     Strategy
}

type timeline struct {
	ticksPerSec int
	srcPeriod   int // ticks per source frame
	outPeriod   int // ticks per output frame
}

func lcm(a, b int) int {
	gcd := func(x, y int) int {
		for y != 0 {
			x, y = y, x%y
		}
		return x
	}
	g := gcd(a, b)
	return a / g * b
}

func rateFPS(r Rate) float64 {
	return float64(r.Num) / float64(r.Den)
}

func (c Config) timeline() timeline {
	srcFPS := c.SourceRate.Num * c.OutputRate.Den
	outFPS := c.OutputRate.Num * c.SourceRate.Den
	tb := lcm(srcFPS, outFPS)
	return timeline{
		ticksPerSec: tb,
		srcPeriod:   tb / srcFPS,
		outPeriod:   tb / outFPS,
	}
}

func ms(ticks, ticksPerSec int) float64 {
	return float64(ticks) / float64(ticksPerSec) * 1000.0
}

// round6 mirrors the six-decimal presentation used by the CSV layer while
// keeping the in-memory model in exact float arithmetic.
func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

// BuildGaps converts the set of missing nominal source indices into the
// contiguous-gap records of the report.
func BuildGaps(missing map[int]bool, srcPeriod int) []Gap {
	if len(missing) == 0 {
		return nil
	}
	var indices []int
	for idx := range missing {
		indices = append(indices, idx)
	}
	for i := 0; i < len(indices); i++ {
		for j := i + 1; j < len(indices); j++ {
			if indices[j] < indices[i] {
				indices[i], indices[j] = indices[j], indices[i]
			}
		}
	}
	var gaps []Gap
	for start := 0; start < len(indices); {
		end := start + 1
		for end < len(indices) && indices[end] == indices[end-1]+1 {
			end++
		}
		first, last := indices[start], indices[end-1]
		gaps = append(gaps, Gap{
			AfterIndex:             first - 1,
			BeforeIndex:            last + 1,
			StartTicks:             first * srcPeriod,
			EndTicks:               (last + 1) * srcPeriod,
			MissingFrames:          end - start,
			NominalPeriodTicks:     srcPeriod,
			FirstRequestedPtsTicks: first * srcPeriod,
			LastRequestedPtsTicks:  last * srcPeriod,
		})
		start = end
	}
	return gaps
}

// Run performs the frame-rate conversion and judder analysis.
func Run(cfg Config) Report {
	tl := cfg.timeline()
	tb := tl.ticksPerSec
	sourceEndTicks := cfg.SourceFrames * tl.srcPeriod
	outputCount := sourceEndTicks / tl.outPeriod

	rep := Report{
		SourceRate:          cfg.SourceRate,
		OutputRate:          cfg.OutputRate,
		TimebaseTicksPerSec: tb,
		SourcePeriodMs:      ms(tl.srcPeriod, tb),
		OutputPeriodMs:      ms(tl.outPeriod, tb),
		Strategy:            cfg.Strategy,
		Gaps:                BuildGaps(cfg.Missing, tl.srcPeriod),
	}

	// present returns the nearest present source frame at or before the
	// nominal index and whether that index itself is present.
	present := func(nominal int) (int, bool) {
		if nominal >= cfg.SourceFrames {
			return cfg.SourceFrames - 1, false
		}
		if !cfg.Missing[nominal] {
			return nominal, true
		}
		idx := nominal - 1
		for idx >= 0 && cfg.Missing[idx] {
			idx--
		}
		return idx, false
	}

	for out := 0; out < outputCount; out++ {
		targetTick := out * tl.outPeriod
		lo := targetTick / tl.srcPeriod
		f := Frame{
			OutputIndex: out,
			PtsTicks:    targetTick,
			PtsMs:       ms(targetTick, tb),
			TargetPtsMs: ms(targetTick, tb),
		}

		switch {
		case targetTick >= sourceEndTicks:
			// Past the last source frame.
			last := cfg.SourceFrames - 1
			f.SourceIndices = []int{last}
			f.SourcePtsMs = []float64{ms(last*tl.srcPeriod, tb)}
			f.EffectivePtsTicks = last * tl.srcPeriod
			f.EffectivePtsMs = ms(f.EffectivePtsTicks, tb)
			f.PhaseErrorMs = ms(f.EffectivePtsTicks-targetTick, tb)
			if cfg.Strategy.Mode == "blend" {
				f.Kind = KindTailHold
				f.Duplicate = true
			} else {
				f.Kind = KindRepeat
				f.Duplicate = true
			}

		case targetTick%tl.srcPeriod == 0 && !cfg.Missing[lo]:
			// Exact hit on an available source frame.
			f.Kind = KindExact
			f.SourceIndices = []int{lo}
			f.SourcePtsMs = []float64{ms(lo*tl.srcPeriod, tb)}
			f.EffectivePtsTicks = targetTick
			f.EffectivePtsMs = ms(targetTick, tb)

		case cfg.Strategy.Mode == "repeat":
			idx, exactPresent := present(lo)
			f.Kind = KindRepeat
			f.Duplicate = true
			f.SourceIndices = []int{idx}
			f.SourcePtsMs = []float64{ms(idx*tl.srcPeriod, tb)}
			f.EffectivePtsTicks = idx * tl.srcPeriod
			f.EffectivePtsMs = ms(f.EffectivePtsTicks, tb)
			f.PhaseErrorMs = ms(f.EffectivePtsTicks-targetTick, tb)
			f.MissingSource = !exactPresent
			if f.MissingSource {
				f.Kind = KindGapHold
			}

		default: // blend
			hi := lo + 1
			if lo >= cfg.SourceFrames-1 {
				last := cfg.SourceFrames - 1
				f.Kind = KindTailHold
				f.Duplicate = true
				f.SourceIndices = []int{last}
				f.SourcePtsMs = []float64{ms(last*tl.srcPeriod, tb)}
				f.EffectivePtsTicks = last * tl.srcPeriod
				f.EffectivePtsMs = ms(f.EffectivePtsTicks, tb)
				f.PhaseErrorMs = ms(f.EffectivePtsTicks-targetTick, tb)
				rep.Frames = append(rep.Frames, f)
				continue
			}
			_, loOK := present(lo)
			hiMissing := hi >= cfg.SourceFrames || cfg.Missing[hi]
			if !loOK || hiMissing {
				idx, _ := present(lo)
				f.Kind = KindGapHold
				f.Duplicate = true
				f.MissingSource = true
				f.SourceIndices = []int{idx}
				f.SourcePtsMs = []float64{ms(idx*tl.srcPeriod, tb)}
				f.EffectivePtsTicks = idx * tl.srcPeriod
				f.EffectivePtsMs = ms(f.EffectivePtsTicks, tb)
				f.PhaseErrorMs = ms(f.EffectivePtsTicks-targetTick, tb)
			} else {
				f.Kind = KindSynthesized
				f.Synthesized = true
				f.SourceIndices = []int{lo, hi}
				f.SourcePtsMs = []float64{
					ms(lo*tl.srcPeriod, tb),
					ms(hi*tl.srcPeriod, tb),
				}
				// Alpha is the target position inside the source period,
				// i.e. the weight of the later source frame in the blend.
				f.Alpha = round6(float64(targetTick-lo*tl.srcPeriod) /
					float64(tl.srcPeriod))
				f.EffectivePtsTicks = targetTick
				f.EffectivePtsMs = ms(targetTick, tb)
			}
		}

		rep.Frames = append(rep.Frames, f)
	}

	rep.JudderCurve = buildCurve(rep.Frames, tb, tl.outPeriod, rep.OutputPeriodMs)
	rep.Summary = buildSummary(rep.Frames, rep.JudderCurve)
	return rep
}

func buildCurve(frames []Frame, tb, idealStepTicks int, idealStep float64) []CurvePoint {
	curve := make([]CurvePoint, len(frames))
	for i, f := range frames {
		p := CurvePoint{
			OutputIndex: f.OutputIndex,
			PtsMs:       f.PtsMs,
			IdealStepMs: idealStep,
			FrameKind:   f.Kind,
		}
		if i == 0 {
			p.Class = ClassBootstrap
			curve[i] = p
			continue
		}
		prev := frames[i-1]
		p.HasStep = true
		// Compute the step in integer timebase ticks so a perfectly even
		// cadence never accumulates floating-point noise; this keeps the
		// "pause vs smooth" classification numerically exact.
		stepTicks := f.EffectivePtsTicks - frames[i-1].EffectivePtsTicks
		p.ActualStepMs = ms(stepTicks, tb)
		p.NormalizedError = float64(stepTicks)/float64(idealStepTicks) - 1
		p.SignedErrorMs = p.NormalizedError * idealStep
		p.Magnitude = math.Abs(p.NormalizedError)

		switch {
		case f.Kind == KindTailHold:
			p.Class = ClassSourceEndHold
		case f.Kind == KindExact && prev.Kind == KindGapHold:
			// First available source frame after a held gap: catch-up jump.
			p.Class = ClassMissingSourceJump
		case f.MissingSource:
			// Every held frame is perceived as the image staying frozen;
			// the first hold inherits a short residual step but is still a
			// pause, only the frame that resumes real motion is a jump.
			p.Class = ClassMissingSourcePause
		case f.Synthesized && p.Magnitude > 0:
			p.Class = ClassInterpolationJudder
		case p.ActualStepMs == 0:
			p.Class = ClassRepeatPause
		case p.Magnitude > 0:
			p.Class = ClassRepeatJump
		default:
			p.Class = ClassSmooth
		}
		curve[i] = p
	}
	return curve
}

func buildSummary(frames []Frame, curve []CurvePoint) Summary {
	s := Summary{OutputFrames: len(frames)}
	for _, f := range frames {
		switch f.Kind {
		case KindExact:
			s.ExactFrames++
		case KindSynthesized:
			s.SynthesizedFrames++
		case KindGapHold:
			s.GapHoldFrames++
		case KindTailHold:
			s.SourceEndHoldFrames++
		}
		if f.Duplicate {
			s.DuplicateFrames++
		}
		if f.MissingSource {
			s.MissingSourceFrames++
		}
	}

	judderClasses := map[string]bool{
		ClassRepeatPause:         true,
		ClassRepeatJump:          true,
		ClassMissingSourcePause:  true,
		ClassMissingSourceJump:   true,
		ClassInterpolationJudder: true,
	}
	var sqSum float64
	rmsN := 0
	for _, p := range curve {
		switch p.Class {
		case ClassRepeatPause:
			s.RepeatPauseFrames++
		case ClassMissingSourcePause:
			s.MissingSourcePauseFrames++
		case ClassInterpolationJudder:
			s.InterpolationJudderFrames++
		}
		if judderClasses[p.Class] {
			s.JudderFrames++
			if p.Magnitude > s.PeakNormalizedJudder {
				s.PeakNormalizedJudder = p.Magnitude
			}
		}
		// RMS covers every real step, excluding the bootstrap sample and
		// the end-of-source tail hold (no source motion remains there).
		if p.Class != ClassBootstrap && p.Class != ClassSourceEndHold {
			sqSum += p.NormalizedError * p.NormalizedError
			rmsN++
		}
	}
	if rmsN > 0 {
		s.RMSNormalizedJudder = math.Sqrt(sqSum / float64(rmsN))
	}
	return s
}

// Compare builds the before/after delta.
func Compare(before, after Report) Comparison {
	return Comparison{
		Before: before,
		After:  after,
		Delta: Delta{
			JudderFramesDelta:         after.Summary.JudderFrames - before.Summary.JudderFrames,
			RepeatPauseDelta:          after.Summary.RepeatPauseFrames - before.Summary.RepeatPauseFrames,
			MissingSourcePauseDelta:   after.Summary.MissingSourcePauseFrames - before.Summary.MissingSourcePauseFrames,
			InterpolationJudderDelta:  after.Summary.InterpolationJudderFrames - before.Summary.InterpolationJudderFrames,
			RMSNormalizedJudderDelta:  after.Summary.RMSNormalizedJudder - before.Summary.RMSNormalizedJudder,
			PeakNormalizedJudderDelta: after.Summary.PeakNormalizedJudder - before.Summary.PeakNormalizedJudder,
		},
	}
}

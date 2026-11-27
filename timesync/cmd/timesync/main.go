// Command timesync demonstrates the separated time-remapping / audio
// resampling model and prints a segment-by-segment audit report.
package main

import (
	"fmt"
	"os"

	"timesync/audio"
	"timesync/audit"
	"timesync/remap"
	"timesync/subs"
)

func main() {
	const (
		srcRate = 16000
		outRate = 16000
	)

	// Piecewise curve spanning the full required range:
	// 1x, 0.2x, 0.5x, 2x, 4x and a linear ramp back down. Rates are
	// playback rates; durations below are in source seconds.
	curve, err := remap.NewCurve([]remap.Segment{
		{S0: 0, S1: 2, RateStart: 1.0, RateEnd: 1.0},
		{S0: 2, S1: 4, RateStart: 0.2, RateEnd: 0.2},
		{S0: 4, S1: 6, RateStart: 0.5, RateEnd: 0.5},
		{S0: 6, S1: 8, RateStart: 2.0, RateEnd: 2.0},
		{S0: 8, S1: 10, RateStart: 4.0, RateEnd: 4.0},
		{S0: 10, S1: 12, RateStart: 4.0, RateEnd: 0.5},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	source := audio.GenerateSine(220, curve.SourceDuration(), srcRate)
	plan := remap.SamplePlan{
		Curve:            curve,
		SourceRateHz:     srcRate,
		OutputRateHz:     outRate,
		SourceFrameCount: len(source),
	}

	linear := audio.LinearResampler{}.Resample(source, plan)
	tsm := audio.NewTSMResampler().Resample(source, plan)

	events := buildWordEvents(curve)
	mapped, err := subs.Remap(events, curve)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	rep := audit.Run(audit.Inputs{
		Plan:         plan,
		Source:       source,
		LinearOutput: linear,
		TSMOutput:    tsm,
		SourceEvents: events,
		MappedEvents: mapped,
	})

	printReport(rep, plan, mapped, len(source), len(linear), len(tsm))
	if !rep.Pass {
		os.Exit(1)
	}
}

// buildWordEvents places one short word-level cue inside every segment so
// the audit can prove each cue is mapped through its own segment.
func buildWordEvents(curve *remap.Curve) []subs.Event {
	words := []string{"you", "said", "the", "speed", "keeps", "changing"}
	var events []subs.Event
	for i, seg := range curve.Segments() {
		start := seg.S0 + 0.25*(seg.S1-seg.S0)
		end := seg.S0 + 0.75*(seg.S1-seg.S0)
		events = append(events, subs.Event{
			ID:    fmt.Sprintf("w%d", i+1),
			Text:  words[i],
			Start: start,
			End:   end,
		})
	}
	return events
}

func printReport(rep *audit.Report, plan remap.SamplePlan, mapped []subs.MappedEvent, srcN, linN, tsmN int) {
	fmt.Println("== time remapping / audio resampling separation audit ==")
	fmt.Printf("source: %.3fs (%d frames) -> output timeline: %.3fs (%d frames)\n\n",
		plan.Curve.SourceDuration(), srcN, plan.Curve.OutputDuration(), plan.OutputFrameCount())

	fmt.Println("per-segment checks:")
	fmt.Println(" seg | src interval (s) | out interval (s) | rate in->out | src smp | out smp exp/obs | map^-1 err | ok")
	for _, s := range rep.Segments {
		fmt.Printf("  %d  | %5.2f -> %5.2f | %6.2f -> %6.2f | %3.1f -> %3.1f | %6d | %6d / %-6d | %7.2fus | %v\n",
			s.Index, s.SourceStart, s.SourceEnd, s.OutputStart, s.OutputEnd,
			s.RateStart, s.RateEnd, s.SourceSamples,
			s.ExpectedOutSamples, s.ObservedOutSamples,
			s.MapInverseMaxErr*1e6, s.LocalRateCheck)
	}

	fmt.Println("\nglobal-offset rejection:")
	fmt.Println(" ", rep.Offset.Message)

	fmt.Println("\npitch (source 220 Hz tone):")
	fmt.Printf("  source %.1f Hz | linear-resample %.1f Hz (drift %+.1f%%) | TSM %.1f Hz (drift %+.1f%%) | pitch preserved: %v\n",
		rep.Pitch.SourcePitchHz,
		rep.Pitch.LinearOutputPitchHz, (rep.Pitch.LinearDriftRatio-1)*100,
		rep.Pitch.TSMOutputPitchHz, (rep.Pitch.TSMDriftRatio-1)*100,
		rep.Pitch.TSMPitchPreserved)

	fmt.Println("\nframe counts:", linN, "linear,", tsmN, "TSM, required", plan.OutputFrameCount())

	fmt.Println("\nword-level subtitles:")
	fmt.Println(" ", rep.SubtitleDetail)
	for _, ev := range mapped {
		fmt.Printf("  %-4s src %.2f-%.2f -> out %.2f-%.2f | drift %+.3fs local | text %q\n",
			ev.ID, ev.Start, ev.End, ev.StartOut, ev.EndOut,
			ev.StartOut-ev.Start, ev.Text)
	}

	fmt.Println("\nfindings:")
	if len(rep.Findings) == 0 {
		fmt.Println("  none")
	}
	for _, f := range rep.Findings {
		fmt.Printf("  [%s] %s: %s\n", f.Severity, f.Where, f.Message)
	}
	fmt.Printf("\nOVERALL: %v\n", rep.Pass)
}

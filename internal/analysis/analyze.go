package analysis

import (
	"fmt"
	"math"

	"resampleforensics/internal/dsp"
)

// Analyze compares a native-rate source with a converted destination. All
// comparisons are latency-resolved before metrics are computed.
func Analyze(source []float64, sourceRate int, destination []float64, destinationRate int, opts Options) (Report, error) {
	if sourceRate <= 0 || destinationRate <= 0 {
		return Report{}, fmt.Errorf("sample rates must be positive")
	}
	if len(source) < sourceRate/10 || len(destination) < destinationRate/10 {
		return Report{}, fmt.Errorf("each file must contain at least 100 ms of audio")
	}
	defaults := DefaultOptions(sourceRate, destinationRate)
	if opts.CutoffHz == 0 {
		opts.CutoffHz = defaults.CutoffHz
	}
	if opts.ReferenceHalfWidth == 0 {
		opts.ReferenceHalfWidth = defaults.ReferenceHalfWidth
	}
	if opts.FFTSize == 0 {
		opts.FFTSize = defaults.FFTSize
	}
	if opts.MaxTransients == 0 {
		opts.MaxTransients = defaults.MaxTransients
	}
	if opts.BoundaryWindowMs == 0 {
		opts.BoundaryWindowMs = defaults.BoundaryWindowMs
	}
	if opts.MinCoherence == 0 {
		opts.MinCoherence = defaults.MinCoherence
	}

	resampler, err := dsp.NewSincResampler(sourceRate, destinationRate, opts.CutoffHz, opts.ReferenceHalfWidth)
	if err != nil {
		return Report{}, err
	}
	ref := resampler.Resample(source, dsp.BoundaryReflect)
	align := Align(ref, destination, destinationRate)
	var warnings []string
	if align.Confidence < 0.55 {
		warnings = append(warnings, "alignment correlation is weak; latency and transient shifts may be unreliable")
	}
	if align.SearchWidth > 0 && math.Abs(align.LagSamples) >= float64(align.SearchWidth) {
		warnings = append(warnings, "latency estimate reached the search limit; inspect duration and leading samples")
	}

	report := Report{
		Source:      streamInfo(source, sourceRate),
		Destination: streamInfo(destination, destinationRate),
		Conversion:  fmt.Sprintf("%d Hz -> %d Hz", sourceRate, destinationRate),
		Alignment: AlignmentReport{
			LagSamples: align.LagSamples,
			LagMS:      align.LagSamples / float64(destinationRate) * 1000,
			Confidence: align.Confidence,
			Method:     "envelope correlation followed by waveform refinement on the strongest transient",
		},
		Warnings: warnings,
	}
	tr, trWarnings, trErr := estimateTransfer(ref, destination, destinationRate, opts, align.IntegerLag)
	report.Warnings = append(report.Warnings, trWarnings...)
	if trErr != nil {
		report.Warnings = append(report.Warnings, "transfer estimate unavailable: "+trErr.Error())
		report.GroupDelay = GroupDelayReport{CoherenceGate: opts.MinCoherence, AggregateSpectrumUsed: false}
		report.Impulse = ImpulseReport{Symmetry: math.NaN(), PreEnergyDB: math.NaN()}
	} else {
		report.GroupDelay = groupDelayBands(tr, destinationRate, align.IntegerLag, opts)
		report.Impulse = impulseMetrics(tr, destinationRate, align.IntegerLag)
	}
	report.Transients = analyzeTransients(source, sourceRate, ref, destination, destinationRate, align.IntegerLag, opts)
	targetHalf := opts.ReferenceHalfWidth
	if !math.IsNaN(report.Impulse.Support60Samples) {
		targetHalf = int(math.Round(report.Impulse.Support60Samples / 2 * float64(sourceRate) / float64(destinationRate)))
	}
	report.Boundary = boundaryReport(source, sourceRate, destinationRate, destination,
		align.IntegerLag, ref, opts, report.GroupDelay.EffectiveBandwidthHz, targetHalf)
	report.Attribution = attribute(report)
	return report, nil
}

func streamInfo(x []float64, rate int) StreamInfo {
	var peak, sum float64
	for _, v := range x {
		if a := math.Abs(v); a > peak {
			peak = a
		}
		sum += v * v
	}
	rms := 0.0
	if len(x) > 0 {
		rms = math.Sqrt(sum / float64(len(x)))
	}
	return StreamInfo{
		SampleRate: rate, Samples: len(x),
		DurationMS: float64(len(x)) / float64(rate) * 1000,
		Peak:       peak, RMS: rms,
	}
}

func analyzeTransients(source []float64, sourceRate int, ref, dst []float64, dstRate, lag int, opts Options) TransientSummary {
	points := detectTransients(source, sourceRate, opts.MaxTransients)
	summary := TransientSummary{DetectedCount: len(points)}
	var ratios, riseRatios, leads, tails, srcWidths, refWidths, dstWidths []float64
	for _, point := range points {
		srcCenter := int(math.Round(point.timeSec * float64(sourceRate)))
		refCenter := int(math.Round(point.timeSec * float64(dstRate)))
		dstCenter := refCenter + lag
		srcM := measureTransient(source, sourceRate, srcCenter)
		refM := measureTransient(ref, dstRate, refCenter)
		dstM := measureTransient(dst, dstRate, dstCenter)
		if !srcM.valid || !refM.valid || !dstM.valid {
			continue
		}
		ratio := math.NaN()
		if refM.width10MS > 0 {
			ratio = dstM.width10MS / refM.width10MS
			ratios = append(ratios, ratio)
		}
		riseRatio := math.NaN()
		if refM.riseMS > 0 {
			riseRatio = dstM.riseMS / refM.riseMS
			riseRatios = append(riseRatios, riseRatio)
		}
		lead := dstM.lead50MS - refM.lead50MS
		tail := dstM.tail50MS - refM.tail50MS
		leads = append(leads, lead)
		tails = append(tails, tail)
		srcWidths = append(srcWidths, srcM.width10MS)
		refWidths = append(refWidths, refM.width10MS)
		dstWidths = append(dstWidths, dstM.width10MS)
		summary.Events = append(summary.Events, TransientEvent{
			TimeSec:       point.timeSec,
			SourceWidthMS: srcM.width10MS, RefWidthMS: refM.width10MS,
			DstWidthMS: dstM.width10MS, WidthRatio: ratio,
			RefRiseMS: refM.riseMS, DstRiseMS: dstM.riseMS, RiseRatio: riseRatio,
			LeadShiftMS: lead, TailShiftMS: tail,
			LeadPhaseDeg: wrapDegrees((dstM.leadPhase - refM.leadPhase) * 180 / math.Pi),
			TailPhaseDeg: wrapDegrees((dstM.tailPhase - refM.tailPhase) * 180 / math.Pi),
			RefPrePostDB: refM.prePostDB, DstPrePostDB: dstM.prePostDB,
			SourcePrePostDB: srcM.prePostDB,
		})
	}
	if len(ratios) > 0 {
		summary.MedianWidthRatio = dsp.Median(ratios)
		summary.P90WidthRatio = dsp.Quantile(ratios, 0.90)
		summary.MedianRiseRatio = dsp.Median(riseRatios)
		summary.MedianLeadShiftMS = dsp.Median(leads)
		summary.MedianTailShiftMS = dsp.Median(tails)
	}
	if len(srcWidths) > 0 {
		srcMed, refMed, dstMed := dsp.Median(srcWidths), dsp.Median(refWidths), dsp.Median(dstWidths)
		tolerance := math.Max(0.12, 0.06*srcMed)
		summary.SourceAlreadyBroad = srcMed >= 4.0 && math.Abs(dstMed-srcMed) <= tolerance
		summary.ConverterWidening = (summary.MedianWidthRatio > 1.06 && dstMed-refMed > 0.12) ||
			summary.MedianRiseRatio > 1.10
	}
	return summary
}

func attribute(r Report) Attribution {
	a := Attribution{Classification: "inconclusive", Confidence: 0.45}
	limitations := []string{
		"the source file is treated as the content reference; defects already encoded into the 44.1 kHz file can only be reported, not repaired",
		"filter length is an effective support derived from the observed response, not a unique converter design parameter",
	}
	var evidence []string
	phaseBad := r.GroupDelay.PhaseCurveIsReliable &&
		(!math.IsNaN(r.GroupDelay.RippleMS) && r.GroupDelay.RippleMS > 0.30 ||
			r.GroupDelay.PhaseLinearityRMSDeg > 5)
	boundaryBad := false
	boundaryBest := math.NaN()
	if !r.Boundary.Indeterminate {
		for _, score := range r.Boundary.Scores {
			if score.Mode == r.Boundary.BestMode && score.HalfWidth == r.Boundary.BestHalfWidth {
				boundaryBest = score.Combined
				break
			}
		}
		boundaryBad = !math.IsNaN(boundaryBest) &&
			boundaryBest > math.Max(0.10, 4*r.Boundary.InteriorNRMSE)
	}
	if r.Transients.DetectedCount < 2 {
		evidence = append(evidence, "too few clean transients were found for a stable statistical comparison")
	} else if r.Transients.ConverterWidening {
		a.Classification = "resampler"
		a.Confidence = dsp.Clamp(0.60+0.6*(r.Transients.MedianWidthRatio-1), 0.60, 0.95)
		evidence = append(evidence, fmt.Sprintf("median width ratio %.3f, rise-time ratio %.3f", r.Transients.MedianWidthRatio, r.Transients.MedianRiseRatio))
	} else if r.Transients.SourceAlreadyBroad {
		a.Classification = "source_content"
		a.Confidence = 0.70
		evidence = append(evidence, "the 44.1 kHz source already has similarly broad transients; conversion adds no stable width increase")
	} else {
		if r.Transients.DetectedCount >= 2 &&
			r.Transients.MedianWidthRatio <= 1.05 &&
			r.Transients.MedianRiseRatio <= 1.05 {
			a.Classification = "source_content"
			a.Confidence = 0.65
			evidence = append(evidence, "conversion does not add a stable transient-width or rise-time increase")
		}
		evidence = append(evidence, fmt.Sprintf("median width ratio %.3f, rise-time ratio %.3f", r.Transients.MedianWidthRatio, r.Transients.MedianRiseRatio))
	}
	if a.Classification != "resampler" && (phaseBad || boundaryBad) {
		a.Classification = "resampler"
		a.Confidence = math.Max(a.Confidence, 0.62)
		if phaseBad {
			evidence = append(evidence, "coherent phase distortion exceeds the time-domain reporting threshold")
		}
		if boundaryBad {
			evidence = append(evidence, "edge-only distortion exceeds the interior residual")
		}
	}
	if !r.Boundary.Indeterminate && r.Boundary.BestMargin > 0.08 {
		evidence = append(evidence, fmt.Sprintf("edge error best matches %s boundary extension (margin %.1f%%)", r.Boundary.BestMode, 100*r.Boundary.BestMargin))
	}
	if r.Impulse.LinearPhaseLikely {
		evidence = append(evidence, fmt.Sprintf("impulse response is approximately symmetric; 60 dB effective span is %.2f ms", r.Impulse.Support60DBMS))
	} else if !math.IsNaN(r.Impulse.Symmetry) {
		evidence = append(evidence, fmt.Sprintf("impulse response is asymmetric (symmetry %.2f), consistent with minimum-phase/IIR behavior", r.Impulse.Symmetry))
	}
	if r.GroupDelay.PhaseCurveIsReliable && !math.IsNaN(r.GroupDelay.RippleMS) && r.GroupDelay.RippleMS > 0.30 {
		evidence = append(evidence, fmt.Sprintf("coherent passband group delay ripples by %.2f ms (p95-p5)", r.GroupDelay.RippleMS))
	}
	if boundaryBad {
		evidence = append(evidence, "boundary-region error is materially above the interior residual")
	}
	if len(evidence) == 0 {
		evidence = append(evidence, "no stable discriminator exceeded its reporting threshold")
	}
	a.Evidence = evidence
	a.Limitations = limitations
	return a
}

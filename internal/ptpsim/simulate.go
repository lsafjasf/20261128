package ptpsim

import (
	"math"
	"time"
)

const tick = time.Millisecond

type recorderSpec struct {
	id        string
	ppm       float64
	asymmetry time.Duration
}

func simulate(cfg Config) Report {
	timeline := buildTimeline(cfg)
	events := buildEvents(cfg)
	buffer := simulateBuffer(cfg)
	lossWindow := cfg.RelockAt - cfg.MasterStop
	if lossWindow < 0 {
		lossWindow = 0
	}

	specs := []recorderSpec{
		{id: recorderA, ppm: cfg.ClockPPM.A, asymmetry: cfg.PathAsymmetry.A},
		{id: recorderB, ppm: cfg.ClockPPM.B, asymmetry: cfg.PathAsymmetry.B},
	}
	recorders := make([]RecorderMetrics, 0, len(specs))
	for _, spec := range specs {
		recorders = append(recorders, runRecorder(cfg, spec, lossWindow))
	}

	counts := CountMetrics{
		A:        recorders[0].WrittenSamples,
		B:        recorders[1].WrittenSamples,
		Delta:    recorders[1].WrittenSamples - recorders[0].WrittenSamples,
		AbsDelta: absInt(recorders[1].WrittenSamples - recorders[0].WrittenSamples),
	}
	timestamp := timestampEvidence(cfg, recorders)
	resampling := resamplingEvidence(cfg, recorders)
	reorder := reorderEvidence(buffer)

	return Report{
		Config:     cfg,
		Timeline:   timeline,
		LossWindow: lossWindow,
		Messages:   countMessages(events),
		Events:     events,
		Recorders:  recorders,
		Counts:     counts,
		Timestamp:  timestamp,
		Resampling: resampling,
		Reorder:    reorder,
		Buffer:     buffer,
	}
}

func runRecorder(cfg Config, spec recorderSpec, lossWindow time.Duration) RecorderMetrics {
	var written float64
	var holdoverSamples float64
	frozenSteps := 0

	for elapsed := time.Duration(0); elapsed < cfg.Duration; elapsed += tick {
		end := elapsed + tick
		if end > cfg.Duration {
			end = cfg.Duration
		}
		duration := end - elapsed
		loss := overlap(elapsed, end, cfg.MasterStop, cfg.RelockAt)
		locked := duration - loss

		lockRate := cfg.SampleRate
		lossRate := cfg.SampleRate * (1 + spec.ppm/1e6)
		if !cfg.Resampler.Enabled {
			lockRate = lossRate
		}
		written += lockRate * locked.Seconds()
		localLossSamples := cfg.SampleRate * (1 + spec.ppm/1e6) * loss.Seconds()
		written += localLossSamples
		holdoverSamples += localLossSamples
		if loss > 0 {
			frozenSteps += int(math.Ceil(loss.Seconds() / tick.Seconds()))
		}
	}

	if cfg.Resampler.Enabled && cfg.Resampler.CatchUpAfter {
		ideal := cfg.SampleRate * lossWindow.Seconds()
		written += ideal - holdoverSamples
	}

	offsetError := spec.asymmetry / 2
	return RecorderMetrics{
		ID:                    spec.id,
		ClockPPM:              spec.ppm,
		WrittenSamples:        int(math.Round(written)),
		HoldoverSamples:       holdoverSamples,
		ResamplerFrozenSteps:  frozenSteps,
		TimestampOffsetError:  offsetError,
		TimestampErrorSamples: float64(offsetError) * cfg.SampleRate / float64(time.Second),
	}
}

func timestampEvidence(cfg Config, recs []RecorderMetrics) TimestampEvidence {
	errA := recs[0].TimestampOffsetError
	errB := recs[1].TimestampOffsetError
	skew := absDuration(errA - errB)
	return TimestampEvidence{
		Skew:        skew,
		SkewSamples: float64(skew) * cfg.SampleRate / float64(time.Second),
		CountEffect: 0,
		Explanation: "Delay-request asymmetry changes PTP offset estimates and timestamps only; no packet or sample is added or removed.",
	}
}

func resamplingEvidence(cfg Config, recs []RecorderMetrics) ResamplingEvidence {
	residual := int(math.Round(math.Abs(recs[0].HoldoverSamples - recs[1].HoldoverSamples)))
	return ResamplingEvidence{
		Enabled:          cfg.Resampler.Enabled,
		FrozenDuringLoss: cfg.Resampler.Enabled && residual > 0,
		CountEffect:      residual,
		Explanation:      "During loss of lock the ASRC ratio is not updated, so output follows the local oscillator instead of the nominal PTP sample rate.",
	}
}

func reorderEvidence(buffer BufferMetrics) ReorderEvidence {
	return ReorderEvidence{
		Inversions:     buffer.ArrivalInversions,
		NetSampleDelta: buffer.NetSampleDelta,
		CountEffect:    absInt(buffer.NetSampleDelta),
		Explanation:    "Sequence reassembly restores packet order; reordering alone changes neither sample count nor sample values.",
	}
}

func overlap(start, end, windowStart, windowEnd time.Duration) time.Duration {
	left := start
	if windowStart > left {
		left = windowStart
	}
	right := end
	if windowEnd < right {
		right = windowEnd
	}
	if right <= left {
		return 0
	}
	return right - left
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func absDuration(v time.Duration) time.Duration {
	if v < 0 {
		return -v
	}
	return v
}

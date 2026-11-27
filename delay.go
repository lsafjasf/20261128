package captionsession

import "time"

// LatencyEstimator tracks the end-to-end delay between the moment speech
// happened on the media timeline (endPTS) and the moment its caption event
// arrives at the server (wall time). The reported ~2s caption lag is simply
// the steady-state value of this estimator. An exponential moving average
// smooths jitter while a drift clamp rejects wild jumps.
type LatencyEstimator struct {
	alpha      float64
	maxDriftMS int64
	ems        float64
	samples    int
	frozen     bool
}

// Default estimator tuning: alpha 0.2 gives ~10 samples to converge, and the
// per-sample drift is clamped to 5s to ignore one-off clock glitches.
const (
	defaultAlpha      = 0.2
	defaultMaxDriftMS = 5000
)

// NewLatencyEstimator builds an estimator with default smoothing.
func NewLatencyEstimator() *LatencyEstimator {
	return &LatencyEstimator{alpha: defaultAlpha, maxDriftMS: defaultMaxDriftMS}
}

// NewLatencyEstimatorWith allows tests to tune smoothing and drift clamp.
func NewLatencyEstimatorWith(alpha float64, maxDriftMS int64) *LatencyEstimator {
	return &LatencyEstimator{alpha: alpha, maxDriftMS: maxDriftMS}
}

// Observe feeds one (event wall time, media endPTS) pair, both in
// milliseconds. Non-positive measured delay is ignored.
func (e *LatencyEstimator) Observe(wallTimeMS, endPTSMS int64) {
	if e.frozen {
		return
	}
	d := wallTimeMS - endPTSMS
	if d <= 0 {
		return
	}
	if e.samples == 0 {
		e.ems = float64(d)
		e.samples = 1
		return
	}
	d64 := float64(d)
	if diff := d64 - e.ems; diff > float64(e.maxDriftMS) {
		d64 = e.ems + float64(e.maxDriftMS)
	} else if diff < -float64(e.maxDriftMS) {
		d64 = e.ems - float64(e.maxDriftMS)
	}
	e.ems = e.alpha*d64 + (1-e.alpha)*e.ems
	e.samples++
}

// Freeze pins the estimate to its current value so subsequent observations
// are ignored. Intended for deterministic tests and operator overrides.
func (e *LatencyEstimator) Freeze() { e.frozen = true }

// EstimateMS returns the smoothed end-to-end pipeline delay in milliseconds.
// Before any sample, the caller-provided assumed delay is used.
func (e *LatencyEstimator) EstimateMS(assumedDelayMS int64) int64 {
	if e.samples == 0 {
		return assumedDelayMS
	}
	return int64(e.ems + 0.5)
}

// Samples returns the number of accepted observations.
func (e *LatencyEstimator) Samples() int { return e.samples }

// Playhead converts the current wall clock into the media PTS currently
// being spoken: wallTime - pipelineDelay. All scheduling decisions are made
// against this value rather than arrival time, which cancels the constant
// caption lag.
func Playhead(wallTimeMS, estimatedDelayMS int64) int64 {
	p := wallTimeMS - estimatedDelayMS
	if p < 0 {
		return 0
	}
	return p
}

// DisplayTime is convenience for clients: the wall time at which a segment
// ending at endPTS should be presented to stay in sync with the audio.
func DisplayTime(endPTSMS, estimatedDelayMS int64) time.Time {
	return time.UnixMilli(endPTSMS + estimatedDelayMS)
}

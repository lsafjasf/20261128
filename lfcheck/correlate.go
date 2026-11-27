package lfcheck

import (
	"fmt"
	"math"
)

// PairResult describes one low-band channel pair. A positive lag means B
// arrives later than A.
type PairResult struct {
	A               string  `json:"a"`
	B               string  `json:"b"`
	ZeroLag         float64 `json:"zero_lag_correlation"`
	Best            float64 `json:"best_correlation"`
	LagSamples      int     `json:"best_lag_samples"`
	LagMS           float64 `json:"best_lag_ms"`
	LagAtSearchEdge bool    `json:"lag_at_search_edge"`
	// GainDB is 20*log10 of the least-squares gain from A to B at the best lag.
	// A negative value means B is quieter. Pearson correlation does not by
	// itself imply a routing gain.
	GainDB        float64 `json:"gain_db"`
	ReverseGainDB float64 `json:"reverse_gain_db"`
	LagReliable   bool    `json:"lag_reliable"`
}

func comparePair(a, b []float64, sampleRate int, maxLag float64) (PairResult, error) {
	result := PairResult{}
	if len(a) != len(b) || len(a) == 0 {
		return result, fmt.Errorf("channel lengths differ or are empty")
	}
	if sampleRate <= 0 {
		return result, fmt.Errorf("invalid sample rate %d", sampleRate)
	}
	maxSamples := int(math.Round(float64(maxLag) * float64(sampleRate)))
	if maxSamples > len(a)/3 {
		maxSamples = len(a) / 3
	}
	if maxSamples < 1 {
		return result, fmt.Errorf("recording is too short for correlation")
	}

	result.ZeroLag, _, _, _ = pearsonLag(a, b, 0)
	step := sampleRate / 3000
	if step < 1 {
		step = 1
	}
	ca := decimate(a, step)
	cb := decimate(b, step)
	coarseMax := maxSamples / step
	bestLag := 0
	bestCorr := 0.0
	for lag := -coarseMax; lag <= coarseMax; lag++ {
		corr, _, _, _ := pearsonLag(ca, cb, lag)
		if math.Abs(corr) > math.Abs(bestCorr) {
			bestCorr, bestLag = corr, lag*step
		}
	}

	// Refine the coarse estimate on the original sample grid. Search only one
	// decimation step either side to avoid selecting a different period of a
	// narrow-band waveform.
	refine := step
	if coarseMax*step < maxSamples {
		refine = step
	}
	refineStart := bestLag - refine
	refineEnd := bestLag + refine
	if refineStart < -maxSamples {
		refineStart = -maxSamples
	}
	if refineEnd > maxSamples {
		refineEnd = maxSamples
	}
	bestLag = clampInt(bestLag, -maxSamples, maxSamples)
	bestCorr, _, _, _ = pearsonLag(a, b, bestLag)
	for lag := refineStart; lag <= refineEnd; lag++ {
		corr, _, _, _ := pearsonLag(a, b, lag)
		if math.Abs(corr) > math.Abs(bestCorr) {
			bestCorr, bestLag = corr, lag
		}
	}

	_, energyA, energyB, cross := pearsonLag(a, b, bestLag)
	if energyA > 0 {
		gain := cross / energyA
		result.GainDB = dbfs(math.Abs(gain))
	}
	if energyB > 0 {
		gain := cross / energyB
		result.ReverseGainDB = dbfs(math.Abs(gain))
	}
	result.Best = bestCorr
	result.LagSamples = bestLag
	result.LagMS = 1000 * float64(bestLag) / float64(sampleRate)
	if bestLag == 0 {
		result.LagMS = 0
	}
	result.LagReliable = math.Abs(bestCorr) >= 0.30
	result.LagAtSearchEdge = bestLag <= -maxSamples || bestLag >= maxSamples
	return result, nil
}

func decimate(samples []float64, step int) []float64 {
	if step <= 1 {
		return samples
	}
	out := make([]float64, len(samples)/step)
	for i := range out {
		var sum float64
		start := i * step
		for j := 0; j < step; j++ {
			sum += samples[start+j]
		}
		out[i] = sum / float64(step)
	}
	return out
}

// pearsonLag returns Pearson correlation and the covariance terms for the
// overlapping region. Cross is sum((a-meanA)*(b-meanB)).
func pearsonLag(a, b []float64, lag int) (corr, energyA, energyB, cross float64) {
	startA, startB := 0, 0
	length := len(a)
	if lag > 0 {
		startB = lag
		length -= lag
	} else if lag < 0 {
		startA = -lag
		length += lag
	}
	if length < 2 {
		return 0, 0, 0, 0
	}
	segA := a[startA : startA+length]
	segB := b[startB : startB+length]
	meanA := mean(segA)
	meanB := mean(segB)
	for i := range segA {
		da := segA[i] - meanA
		db := segB[i] - meanB
		energyA += da * da
		energyB += db * db
		cross += da * db
	}
	if energyA > 0 && energyB > 0 {
		corr = cross / math.Sqrt(energyA*energyB)
		if corr > 1 {
			corr = 1
		} else if corr < -1 {
			corr = -1
		}
	}
	return corr, energyA, energyB, cross
}

func mean(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, value := range samples {
		sum += value
	}
	return sum / float64(len(samples))
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

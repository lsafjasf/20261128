package analyze

import (
	"math"
)

type Result struct {
	Name                  string
	SampleRate            int
	PeakSource            float64
	PeakOutput            float64
	PeakDeltaDB           float64
	TransientEnergyRatio  float64
	AttackSlopeRatio      float64
	ResidualRMS           float64
	ResidualPeak          float64
	ResidualTransientRMS  float64
	ResidualTransientPeak float64
	ResidualDC            float64
	ResidualCorrelation   float64
	ResidualZCR           float64
	ResidualHPF           float64
	ResidualRepeatCorr    float64
	ClampedSamples        int
	Onsets                []int
}

type FrameRow struct {
	Frame            int
	StartSample      int
	SourceEnergy     float64
	OutputEnergy     float64
	ResidualEnvelope float64
	ResidualHPF      float64
	TransientWindow  bool
}

func Compare(name string, sampleRate int, reference24, output16 []int32) (Result, []FrameRow) {
	sourceLevel := to16Level(reference24)
	outputLevel := make([]float64, len(output16))
	residual := make([]float64, len(output16))
	clamped := 0
	for i := range output16 {
		outputLevel[i] = float64(output16[i])
		residual[i] = sourceLevel[i] - outputLevel[i]
		if output16[i] == math.MaxInt16 || output16[i] == math.MinInt16 {
			clamped++
		}
	}

	onsets := DetectOnsets(sourceLevel, sampleRate)
	transientMask := make([]bool, len(sourceLevel))
	attackSamples := sampleRate / 200
	bodySamples := sampleRate / 20
	for _, onset := range onsets {
		start := max(0, onset-attackSamples)
		end := min(len(transientMask), onset+bodySamples)
		for sample := start; sample < end; sample++ {
			transientMask[sample] = true
		}
	}

	sourcePeak, outputPeak := peak(sourceLevel), peak(outputLevel)
	sourceEnergy, outputEnergy := windowEnergy(sourceLevel, transientMask), windowEnergy(outputLevel, transientMask)
	sourceSlope, outputSlope := attackSlope(sourceLevel, onsets, attackSamples), attackSlope(outputLevel, onsets, attackSamples)

	result := Result{
		Name:                  name,
		SampleRate:            sampleRate,
		PeakSource:            sourcePeak / float64(math.MaxInt16),
		PeakOutput:            outputPeak / float64(math.MaxInt16),
		PeakDeltaDB:           toDB(outputPeak / sourcePeak),
		TransientEnergyRatio:  toDBSafe(outputEnergy / sourceEnergy),
		AttackSlopeRatio:      toDBSafe(outputSlope / sourceSlope),
		ResidualRMS:           maskedRMS(residual, transientMask) / float64(math.MaxInt16),
		ResidualPeak:          maxAbs(residual) / float64(math.MaxInt16),
		ResidualTransientRMS:  windowRMS(residual, transientMask) / float64(math.MaxInt16),
		ResidualTransientPeak: maskedPeak(residual, transientMask) / float64(math.MaxInt16),
		ResidualDC:            mean(residual) / float64(math.MaxInt16),
		ResidualCorrelation:   correlation(sourceLevel, residual),
		ResidualZCR:           zeroCrossingRate(residual, transientMask),
		ResidualHPF:           maskedRMS(highPassOnePole(residual), transientMask) / float64(math.MaxInt16),
		ResidualRepeatCorr:    1,
		ClampedSamples:        clamped,
		Onsets:                onsets,
	}
	rows := frames(sourceLevel, outputLevel, residual, transientMask, sampleRate/200)
	return result, rows
}

func CompareIndependent(name string, sampleRate int, reference24, output16, independent16 []int32) (Result, []FrameRow) {
	result, rows := Compare(name, sampleRate, reference24, output16)
	independentLevel := make([]float64, len(output16))
	sourceLevel := to16Level(reference24)
	independentResidual := make([]float64, len(output16))
	for i := range output16 {
		independentLevel[i] = float64(independent16[i])
		independentResidual[i] = sourceLevel[i] - independentLevel[i]
	}
	transientMask := make([]bool, len(sourceLevel))
	for _, onset := range result.Onsets {
		start := max(0, onset-sampleRate/200)
		end := min(len(transientMask), onset+sampleRate/20)
		for sample := start; sample < end; sample++ {
			transientMask[sample] = true
		}
	}
	firstResidual := make([]float64, len(output16))
	for i := range output16 {
		firstResidual[i] = sourceLevel[i] - float64(output16[i])
	}
	result.ResidualRepeatCorr = maskedCorrelation(firstResidual, independentResidual, transientMask)
	return result, rows
}

func DetectOnsets(signal []float64, sampleRate int) []int {
	if len(signal) == 0 || sampleRate <= 0 {
		return nil
	}
	var onsets []int
	cooldown := sampleRate / 10
	globalPeak := maxAbs(signal)
	if globalPeak == 0 {
		return nil
	}
	threshold := globalPeak * 0.35
	neighborhood := max(1, sampleRate/400)

	for sample := 0; sample < len(signal); sample++ {
		amplitude := math.Abs(signal[sample])
		if amplitude < threshold || !isLocalPeak(signal, sample, neighborhood) {
			continue
		}
		if len(onsets) > 0 && sample-onsets[len(onsets)-1] < cooldown {
			previous := onsets[len(onsets)-1]
			if amplitude > math.Abs(signal[previous]) {
				onsets[len(onsets)-1] = sample
			}
		} else if len(onsets) == 0 || amplitude > math.Abs(signal[onsets[len(onsets)-1]])*0.72 {
			onsets = append(onsets, sample)
		}
	}
	return onsets
}

func isLocalPeak(signal []float64, index, neighborhood int) bool {
	amplitude := math.Abs(signal[index])
	start := max(0, index-neighborhood)
	end := min(len(signal), index+neighborhood+1)
	for candidate := start; candidate < end; candidate++ {
		if candidate != index && math.Abs(signal[candidate]) > amplitude {
			return false
		}
	}
	return true
}

func to16Level(values []int32) []float64 {
	output := make([]float64, len(values))
	for i, value := range values {
		output[i] = float64(value) / 256.0
	}
	return output
}

func frames(source, output, residual []float64, mask []bool, frameSize int) []FrameRow {
	filtered := highPassOnePole(residual)
	rows := make([]FrameRow, 0, len(source)/frameSize+1)
	for frame, start := 0, 0; start < len(source); frame, start = frame+1, start+frameSize {
		end := min(len(source), start+frameSize)
		rows = append(rows, FrameRow{
			Frame:            frame,
			StartSample:      start,
			SourceEnergy:     rms(source[start:end]),
			OutputEnergy:     rms(output[start:end]),
			ResidualEnvelope: rms(residual[start:end]),
			ResidualHPF:      rms(filtered[start:end]),
			TransientWindow:  mask[start],
		})
	}
	return rows
}

func attackSlope(signal []float64, onsets []int, samples int) float64 {
	total := 0.0
	for _, onset := range onsets {
		start := max(0, onset-samples)
		end := min(len(signal), onset+samples)
		total += (maxAbs(signal[start:end]) - rms(signal[start:onset])) / float64(end-start)
	}
	if len(onsets) == 0 {
		return 0
	}
	return total / float64(len(onsets))
}

func highPassOnePole(signal []float64) []float64 {
	output := make([]float64, len(signal))
	previousInput := 0.0
	previousOutput := 0.0
	alpha := 0.85
	for i, value := range signal {
		output[i] = alpha * (previousOutput + value - previousInput)
		previousInput = value
		previousOutput = output[i]
	}
	return output
}

func windowEnergy(signal []float64, mask []bool) float64 {
	total := 0.0
	count := 0
	for i, value := range signal {
		if mask[i] {
			total += value * value
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func maskedRMS(signal []float64, mask []bool) float64 {
	total := 0.0
	count := 0
	for i, value := range signal {
		if !mask[i] {
			total += value * value
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return math.Sqrt(total / float64(count))
}

func windowRMS(signal []float64, mask []bool) float64 {
	total := 0.0
	count := 0
	for i, value := range signal {
		if mask[i] {
			total += value * value
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return math.Sqrt(total / float64(count))
}

func maskedPeak(signal []float64, mask []bool) float64 {
	maximum := 0.0
	for i, value := range signal {
		if mask[i] && math.Abs(value) > maximum {
			maximum = math.Abs(value)
		}
	}
	return maximum
}

func correlation(left, right []float64) float64 {
	leftMean, rightMean := mean(left), mean(right)
	var covariance, leftVariance, rightVariance float64
	for i := range left {
		lv := left[i] - leftMean
		rv := right[i] - rightMean
		covariance += lv * rv
		leftVariance += lv * lv
		rightVariance += rv * rv
	}
	denominator := math.Sqrt(leftVariance * rightVariance)
	if denominator == 0 {
		return 0
	}
	return covariance / denominator
}

func maskedCorrelation(left, right []float64, mask []bool) float64 {
	leftValues := make([]float64, 0, len(left))
	rightValues := make([]float64, 0, len(right))
	for i := range left {
		if !mask[i] {
			leftValues = append(leftValues, left[i])
			rightValues = append(rightValues, right[i])
		}
	}
	return correlation(leftValues, rightValues)
}

func zeroCrossingRate(signal []float64, mask []bool) float64 {
	crossings := 0
	count := 0
	for i := 1; i < len(signal); i++ {
		if mask[i] {
			continue
		}
		if (signal[i-1] >= 0) != (signal[i] >= 0) {
			crossings++
		}
		count++
	}
	if count == 0 {
		return 0
	}
	return float64(crossings) / float64(count)
}

func rms(signal []float64) float64 {
	if len(signal) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range signal {
		total += value * value
	}
	return math.Sqrt(total / float64(len(signal)))
}

func peak(signal []float64) float64 {
	return maxAbs(signal)
}

func maxAbs(signal []float64) float64 {
	maximum := 0.0
	for _, value := range signal {
		amplitude := math.Abs(value)
		if amplitude > maximum {
			maximum = amplitude
		}
	}
	return maximum
}

func mean(signal []float64) float64 {
	total := 0.0
	for _, value := range signal {
		total += value
	}
	return total / float64(len(signal))
}

func toDB(ratio float64) float64 {
	return 20 * math.Log10(math.Abs(ratio))
}

func toDBSafe(ratio float64) float64 {
	if ratio <= 0 {
		return math.Inf(-1)
	}
	return 10 * math.Log10(ratio)
}

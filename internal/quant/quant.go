package quant

import (
	"math"
	"math/rand/v2"
)

const (
	Int24Max = 1<<23 - 1
	Int24Min = -1 << 23
	Int16Max = math.MaxInt16
	Int16Min = math.MinInt16
	CodeStep = 256
)

func FloatToInt24(samples []float64) []int32 {
	values := make([]int32, len(samples))
	for i, sample := range samples {
		value := int64(math.Round(sample * float64(Int24Max)))
		if value > Int24Max {
			value = Int24Max
		}
		if value < Int24Min {
			value = Int24Min
		}
		values[i] = int32(value)
	}
	return values
}

func Int16ToFloat(values []int32) []float64 {
	samples := make([]float64, len(values))
	scale := 1.0 / float64(Int16Max)
	for i, value := range values {
		samples[i] = float64(value) * scale
	}
	return samples
}

func Truncate(source24 []int32) []int32 {
	output := make([]int32, len(source24))
	for i, value := range source24 {
		output[i] = clamp16(arithmeticShiftRight8(value))
	}
	return output
}

func arithmeticShiftRight8(value int32) int32 {
	if value >= 0 {
		return value / CodeStep
	}
	return -((-value + CodeStep - 1) / CodeStep)
}

func TPDFDither(source24 []int32, seed uint64) []int32 {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	output := make([]int32, len(source24))
	for i, value := range source24 {
		dither := tpdf(rng)
		quantized := int32(math.Floor(float64(value)/CodeStep + 0.5 + dither))
		output[i] = clamp16(quantized)
	}
	return output
}

func ShapedTPDF(source24 []int32, seed uint64) []int32 {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	output := make([]int32, len(source24))
	var previousError, olderError float64
	for i, value := range source24 {
		inputLevel := float64(value) / CodeStep
		dither := tpdf(rng)
		target := inputLevel + dither - (2*previousError - olderError)
		quantized := int32(math.Floor(target + 0.5))
		quantized = clamp16(quantized)
		errorValue := float64(quantized) - target
		olderError = previousError
		previousError = errorValue
		output[i] = quantized
	}
	return output
}

func ApplyHeadroom(source24 []int32, headroomDB float64) ([]int32, float64) {
	peak := 0.0
	for _, value := range source24 {
		amplitude := math.Abs(float64(value))
		if amplitude > peak {
			peak = amplitude
		}
	}
	limit := math.Pow(10, headroomDB/20) * float64(Int24Max)
	gain := 1.0
	if peak > limit {
		gain = limit / peak
	}
	output := make([]int32, len(source24))
	for i, value := range source24 {
		scaled := int64(math.Round(float64(value) * gain))
		if scaled > Int24Max {
			scaled = Int24Max
		}
		if scaled < Int24Min {
			scaled = Int24Min
		}
		output[i] = int32(scaled)
	}
	return output, gain
}

func tpdf(rng *rand.Rand) float64 {
	return (rng.Float64() - 0.5) + (rng.Float64() - 0.5)
}

func clamp16(value int32) int32 {
	if value > Int16Max {
		return Int16Max
	}
	if value < Int16Min {
		return Int16Min
	}
	return value
}

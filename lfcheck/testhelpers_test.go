package lfcheck

import (
	"encoding/binary"
	"math"
)

func synthAudio(sampleRate, seconds int, channels [][]float64) Audio {
	samples := make([][]float64, len(channels))
	for ch := range channels {
		samples[ch] = append([]float64(nil), channels[ch]...)
	}
	return Audio{
		Source:     "synthetic",
		SampleRate: sampleRate,
		Channels:   len(samples),
		Bits:       16,
		Codec:      "PCM",
		Samples:    samples,
	}
}

func tone(n, sampleRate int, phase float64, freqs ...float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		t := float64(i) / float64(sampleRate)
		var value float64
		for j, hz := range freqs {
			value += math.Sin(2*math.Pi*hz*t + phase + float64(j)*0.37)
		}
		out[i] = value / float64(len(freqs))
	}
	return out
}

func add(a, b []float64, gain float64) []float64 {
	out := append([]float64(nil), a...)
	for i := range out {
		out[i] += gain * b[i]
	}
	return out
}

func scale(a []float64, gain float64) []float64 {
	out := make([]float64, len(a))
	for i := range out {
		out[i] = a[i] * gain
	}
	return out
}

func delay(samples []float64, n int) []float64 {
	out := make([]float64, len(samples))
	if n >= 0 {
		copy(out[n:], samples)
	} else {
		copy(out, samples[-n:])
	}
	return out
}

func sixChannels(l, r, c, lfe []float64, n int) Audio {
	silence := make([]float64, n)
	return synthAudio(48000, n/48000, [][]float64{l, r, c, lfe, silence, silence})
}

func hasFinding(result Analysis, code string) bool {
	for _, finding := range result.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func pcm16WAV(channels int, sampleRate int, samples [][]float64) []byte {
	frames := len(samples[0])
	dataSize := frames * channels * 2
	size := 44 + dataSize
	out := make([]byte, size)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(size-8))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1)
	binary.LittleEndian.PutUint16(out[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(sampleRate*channels*2))
	binary.LittleEndian.PutUint16(out[32:34], uint16(channels*2))
	binary.LittleEndian.PutUint16(out[34:36], 16)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataSize))
	pos := 44
	for frame := 0; frame < frames; frame++ {
		for ch := 0; ch < channels; ch++ {
			value := int16(math.Round(samples[ch][frame] * 32767))
			binary.LittleEndian.PutUint16(out[pos:pos+2], uint16(value))
			pos += 2
		}
	}
	return out
}

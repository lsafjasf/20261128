package main

import (
	"encoding/binary"
	"math"
	"os"
)

func float32FromBits(u uint32) float32 { return math.Float32frombits(u) }

// WriteWav writes samples using the source format when possible,
// falling back to 32-bit float to avoid re-quantizing repaired audio.
func WriteWav(path string, w *WavFile, forceFloat bool) error {
	bits := w.SourceBits
	isFloat := w.SourceFloat
	if forceFloat || bits == 0 {
		bits, isFloat = 32, true
	}
	bytesPer := bits / 8
	frames := w.Frames()
	blockAlign := w.Channels * bytesPer
	dataSize := frames * blockAlign

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	audioFmt := uint16(1)
	if isFloat {
		audioFmt = 3
	}
	hdr := make([]byte, 0, 44)
	hdr = append(hdr, "RIFF"...)
	hdr = appendU32(hdr, uint32(36+dataSize))
	hdr = append(hdr, "WAVE"...)
	hdr = append(hdr, "fmt "...)
	hdr = appendU32(hdr, 16)
	hdr = appendU16(hdr, audioFmt)
	hdr = appendU16(hdr, uint16(w.Channels))
	hdr = appendU32(hdr, uint32(w.SampleRate))
	hdr = appendU32(hdr, uint32(w.SampleRate*blockAlign))
	hdr = appendU16(hdr, uint16(blockAlign))
	hdr = appendU16(hdr, uint16(bits))
	hdr = append(hdr, "data"...)
	hdr = appendU32(hdr, uint32(dataSize))
	if _, err := f.Write(hdr); err != nil {
		return err
	}

	buf := make([]byte, blockAlign)
	for i := 0; i < frames; i++ {
		for c := 0; c < w.Channels; c++ {
			encodeSample(buf[c*bytesPer:(c+1)*bytesPer], w.Samples[c][i], bits, isFloat)
		}
		if _, err := f.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func encodeSample(b []byte, v float64, bits int, isFloat bool) {
	if v > 1.0 {
		v = 1.0
	} else if v < -1.0 {
		v = -1.0
	}
	if isFloat {
		binary.LittleEndian.PutUint32(b, math.Float32bits(float32(v)))
		return
	}
	switch bits {
	case 16:
		binary.LittleEndian.PutUint16(b, uint16(int16(math.Round(v*32767))))
	case 24:
		n := int32(math.Round(v * 8388607))
		b[0], b[1], b[2] = byte(n), byte(n>>8), byte(n>>16)
	case 32:
		binary.LittleEndian.PutUint32(b, uint32(int32(math.Round(v*2147483647))))
	}
}

func appendU16(b []byte, v uint16) []byte {
	var t [2]byte
	binary.LittleEndian.PutUint16(t[:], v)
	return append(b, t[:]...)
}

func appendU32(b []byte, v uint32) []byte {
	var t [4]byte
	binary.LittleEndian.PutUint32(t[:], v)
	return append(b, t[:]...)
}

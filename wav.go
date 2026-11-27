package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// WavFile holds decoded audio as per-channel float64 samples in [-1, 1).
type WavFile struct {
	SampleRate  int
	Channels    int
	Samples     [][]float64 // [channel][frame]
	SourceBits  int         // original bit depth (for round-trip writes)
	SourceFloat bool
}

func (w *WavFile) Frames() int {
	if len(w.Samples) == 0 {
		return 0
	}
	return len(w.Samples[0])
}

func ReadWav(path string) (*WavFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var riff [12]byte
	if _, err := io.ReadFull(f, riff[:]); err != nil {
		return nil, fmt.Errorf("read RIFF header: %w", err)
	}
	if string(riff[0:4]) != "RIFF" || string(riff[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF/WAVE file")
	}

	var (
		audioFmt, bits, channels, sampleRate int
		data                                 []byte
		haveFmt, haveData                    bool
	)
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			break
		}
		size := int(binary.LittleEndian.Uint32(hdr[4:8]))
		switch string(hdr[0:4]) {
		case "fmt ":
			buf := make([]byte, size)
			if _, err := io.ReadFull(f, buf); err != nil {
				return nil, err
			}
			audioFmt = int(binary.LittleEndian.Uint16(buf[0:2]))
			channels = int(binary.LittleEndian.Uint16(buf[2:4]))
			sampleRate = int(binary.LittleEndian.Uint32(buf[4:8]))
			bits = int(binary.LittleEndian.Uint16(buf[14:16]))
			haveFmt = true
		case "data":
			data = make([]byte, size)
			if _, err := io.ReadFull(f, data); err != nil {
				return nil, err
			}
			haveData = true
		default:
			if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
				return nil, err
			}
		}
		if size%2 == 1 {
			f.Seek(1, io.SeekCurrent) // chunk pad byte
		}
	}
	if !haveFmt || !haveData {
		return nil, errors.New("missing fmt or data chunk")
	}
	if audioFmt != 1 && audioFmt != 3 {
		return nil, fmt.Errorf("unsupported format tag %d (want 1=PCM or 3=float)", audioFmt)
	}

	w := &WavFile{SampleRate: sampleRate, Channels: channels, SourceBits: bits, SourceFloat: audioFmt == 3}
	w.Samples = make([][]float64, channels)
	bytesPer := bits / 8
	frames := len(data) / (bytesPer * channels)
	for c := range w.Samples {
		w.Samples[c] = make([]float64, frames)
	}
	for i := 0; i < frames; i++ {
		for c := 0; c < channels; c++ {
			off := (i*channels + c) * bytesPer
			w.Samples[c][i] = decodeSample(data[off:off+bytesPer], bits, audioFmt == 3)
		}
	}
	return w, nil
}

func decodeSample(b []byte, bits int, isFloat bool) float64 {
	if isFloat {
		if bits == 32 {
			return float64(float32FromBits(binary.LittleEndian.Uint32(b)))
		}
		return 0
	}
	switch bits {
	case 16:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768.0
	case 24:
		v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
		if v&0x800000 != 0 {
			v |= ^0xFFFFFF
		}
		return float64(v) / 8388608.0
	case 32:
		return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648.0
	}
	return 0
}

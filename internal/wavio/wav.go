package wavio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	MaxInt24 = 1<<23 - 1
	MinInt24 = -1 << 23
)

type PCM struct {
	SampleRate int
	Samples    []float64
}

func ReadMono24(path string) (PCM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PCM{}, err
	}
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return PCM{}, errors.New("not a RIFF/WAVE file")
	}

	pos := 12
	var audio []byte
	channels := 0
	sampleRate := 0
	bits := 0
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if body+size > len(data) {
			return PCM{}, errors.New("truncated WAV chunk")
		}
		if id == "fmt " {
			if size < 16 {
				return PCM{}, errors.New("unsupported fmt chunk")
			}
			format := int(binary.LittleEndian.Uint16(data[body : body+2]))
			channels = int(binary.LittleEndian.Uint16(data[body+2 : body+4]))
			sampleRate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
			bits = int(binary.LittleEndian.Uint16(data[body+14 : body+16]))
			if format != 1 || channels != 1 || bits != 24 {
				return PCM{}, fmt.Errorf("only mono PCM24 is supported, got format=%d channels=%d bits=%d", format, channels, bits)
			}
		}
		if id == "data" {
			audio = data[body : body+size]
		}
		pos = body + size + size%2
	}
	if audio == nil {
		return PCM{}, errors.New("missing data chunk")
	}
	if len(audio)%3 != 0 {
		return PCM{}, errors.New("truncated 24-bit sample")
	}

	samples := make([]float64, len(audio)/3)
	scale := 1.0 / float64(MaxInt24)
	for i := range samples {
		offset := i * 3
		value := int32(audio[offset]) | int32(audio[offset+1])<<8 | int32(audio[offset+2])<<16
		if value&(1<<23) != 0 {
			value |= ^int32(0xffffff)
		}
		samples[i] = float64(value) * scale
	}
	return PCM{SampleRate: sampleRate, Samples: samples}, nil
}

func WriteMono24(path string, pcm PCM) error {
	values := make([]int32, len(pcm.Samples))
	for i, sample := range pcm.Samples {
		values[i] = clamp24(round(sample * float64(MaxInt24)))
	}
	return writePCM(path, pcm.SampleRate, 24, func(w io.Writer) error {
		buf := make([]byte, len(values)*3)
		for i, value := range values {
			unsigned := uint32(value) << 8
			offset := i * 3
			buf[offset] = byte(unsigned >> 8)
			buf[offset+1] = byte(unsigned >> 16)
			buf[offset+2] = byte(unsigned >> 24)
		}
		_, err := w.Write(buf)
		return err
	}, len(values)*3)
}

func WriteMono16(path string, pcm PCM) error {
	values := make([]int16, len(pcm.Samples))
	for i, sample := range pcm.Samples {
		values[i] = int16(clamp16(round(sample * float64(math.MaxInt16))))
	}
	return writePCM(path, pcm.SampleRate, 16, func(w io.Writer) error {
		return binary.Write(w, binary.LittleEndian, values)
	}, len(values)*2)
}

func writePCM(path string, sampleRate, bits int, writeBody func(io.Writer) error, dataBytes int) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	frameBytes := bits / 8
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataBytes))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(sampleRate*frameBytes))
	binary.LittleEndian.PutUint16(header[32:34], uint16(frameBytes))
	binary.LittleEndian.PutUint16(header[34:36], uint16(bits))
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataBytes))
	if _, err := file.Write(header); err != nil {
		return err
	}
	return writeBody(file)
}

func round(value float64) int64 {
	return int64(math.Round(value))
}

func clamp24(value int64) int32 {
	if value > MaxInt24 {
		return MaxInt24
	}
	if value < MinInt24 {
		return MinInt24
	}
	return int32(value)
}

func clamp16(value int64) int {
	if value > math.MaxInt16 {
		return math.MaxInt16
	}
	if value < math.MinInt16 {
		return math.MinInt16
	}
	return int(value)
}

package lfcheck

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

const (
	formatPCM        = 0x0001
	formatIEEEFloat  = 0x0003
	formatExtensible = 0xfffe
)

// Audio is interleaved sample data decoded to normalized float64 values.
// Samples[channel][frame] is used by the analysis code.
type Audio struct {
	Source      string
	SampleRate  int
	Channels    int
	Bits        int
	Codec       string
	ChannelMask uint32
	HasMask     bool
	Samples     [][]float64
}

// Duration returns the duration in seconds.
func (a Audio) Duration() float64 {
	if a.SampleRate <= 0 || len(a.Samples) == 0 {
		return 0
	}
	return float64(len(a.Samples[0])) / float64(a.SampleRate)
}

// ReadWAV decodes a RIFF/WAVE file. It intentionally supports the formats
// useful for program-audio checks without relying on external codecs.
func ReadWAV(path string) (Audio, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Audio{}, fmt.Errorf("read %s: %w", path, err)
	}
	audio, err := ParseWAV(data)
	if err != nil {
		return Audio{}, fmt.Errorf("%s: %w", path, err)
	}
	audio.Source = path
	return audio, nil
}

// ParseWAV decodes an in-memory RIFF/WAVE image.
func ParseWAV(data []byte) (Audio, error) {
	if len(data) < 12 || (string(data[0:4]) != "RIFF" && string(data[0:4]) != "RF64") ||
		string(data[8:12]) != "WAVE" {
		return Audio{}, fmt.Errorf("not a RIFF/WAVE file")
	}

	var info wavFormat
	var pcm []byte
	haveFmt, haveData := false, false
	for pos := 12; pos+8 <= len(data); {
		id := string(data[pos : pos+4])
		size := uint64(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		start := pos + 8
		if size > uint64(len(data)-start) {
			if id == "data" {
				// Truncated data is still analyzable, but not silently treated
				// as complete. The available bytes are decoded.
				size = uint64(len(data) - start)
			} else {
				return Audio{}, fmt.Errorf("chunk %q extends past end of file", id)
			}
		}
		payload := data[start : start+int(size)]
		switch id {
		case "fmt ":
			var err error
			info, err = parseFormat(payload)
			if err != nil {
				return Audio{}, err
			}
			haveFmt = true
		case "data":
			pcm = payload
			haveData = true
		}
		next := start + int(size)
		if size&1 != 0 {
			next++
		}
		if next <= pos {
			break
		}
		pos = next
	}
	if !haveFmt {
		return Audio{}, fmt.Errorf("missing fmt chunk")
	}
	if !haveData {
		return Audio{}, fmt.Errorf("missing data chunk")
	}
	return decodeSamples(pcm, info)
}

type wavFormat struct {
	codec       uint16
	channels    int
	sampleRate  int
	blockAlign  int
	bits        int
	channelMask uint32
	hasMask     bool
}

func parseFormat(payload []byte) (wavFormat, error) {
	if len(payload) < 16 {
		return wavFormat{}, fmt.Errorf("truncated fmt chunk")
	}
	f := wavFormat{
		codec:      binary.LittleEndian.Uint16(payload[0:2]),
		channels:   int(binary.LittleEndian.Uint16(payload[2:4])),
		sampleRate: int(binary.LittleEndian.Uint32(payload[4:8])),
		blockAlign: int(binary.LittleEndian.Uint16(payload[12:14])),
		bits:       int(binary.LittleEndian.Uint16(payload[14:16])),
	}
	if f.channels <= 0 {
		return wavFormat{}, fmt.Errorf("fmt declares zero channels")
	}
	if f.sampleRate <= 0 {
		return wavFormat{}, fmt.Errorf("fmt declares invalid sample rate %d", f.sampleRate)
	}
	if f.bits <= 0 || f.bits > 64 {
		return wavFormat{}, fmt.Errorf("unsupported sample width %d", f.bits)
	}

	if f.codec == formatExtensible {
		if len(payload) < 40 {
			return wavFormat{}, fmt.Errorf("truncated WAVE_FORMAT_EXTENSIBLE fmt chunk")
		}
		f.channelMask = binary.LittleEndian.Uint32(payload[20:24])
		f.hasMask = true
		f.codec = binary.LittleEndian.Uint16(payload[24:26])
	}
	expectedAlign := f.channels * ((f.bits + 7) / 8)
	if f.blockAlign == 0 {
		f.blockAlign = expectedAlign
	}
	if f.blockAlign != expectedAlign {
		return wavFormat{}, fmt.Errorf("block alignment %d does not match %d channels at %d bits",
			f.blockAlign, f.channels, f.bits)
	}
	if f.codec != formatPCM && f.codec != formatIEEEFloat {
		return wavFormat{}, fmt.Errorf("unsupported WAV codec 0x%04x", f.codec)
	}
	if f.codec == formatPCM && f.bits != 8 && f.bits != 16 && f.bits != 24 && f.bits != 32 {
		return wavFormat{}, fmt.Errorf("unsupported PCM sample width %d", f.bits)
	}
	return f, nil
}

func decodeSamples(pcm []byte, f wavFormat) (Audio, error) {
	width := (f.bits + 7) / 8
	frames := len(pcm) / f.blockAlign
	if frames == 0 {
		return Audio{}, fmt.Errorf("data chunk contains no complete frames")
	}
	samples := make([][]float64, f.channels)
	for ch := range samples {
		samples[ch] = make([]float64, frames)
	}
	codec := "PCM"
	if f.codec == formatIEEEFloat {
		codec = "IEEE float"
	}
	audio := Audio{
		SampleRate:  f.sampleRate,
		Channels:    f.channels,
		Bits:        f.bits,
		Codec:       codec,
		ChannelMask: f.channelMask,
		HasMask:     f.hasMask,
		Samples:     samples,
	}

	for frame := 0; frame < frames; frame++ {
		base := frame * f.blockAlign
		for ch := 0; ch < f.channels; ch++ {
			off := base + ch*width
			raw := pcm[off : off+width]
			var value float64
			switch {
			case f.codec == formatIEEEFloat && f.bits == 32:
				value = float64(math.Float32frombits(binary.LittleEndian.Uint32(raw)))
			case f.codec == formatIEEEFloat && f.bits == 64:
				value = math.Float64frombits(binary.LittleEndian.Uint64(raw))
			case f.codec == formatIEEEFloat:
				return Audio{}, fmt.Errorf("unsupported IEEE float width %d", f.bits)
			default:
				value = decodePCM(raw, f.bits)
			}
			if math.IsNaN(value) || math.IsInf(value, 0) {
				value = 0
			}
			samples[ch][frame] = value
		}
	}
	return audio, nil
}

func decodePCM(raw []byte, bits int) float64 {
	switch bits {
	case 8:
		return (float64(raw[0]) - 128) / 128
	case 16:
		return float64(int16(binary.LittleEndian.Uint16(raw))) / 32768
	case 24:
		value := int32(raw[0]) | int32(raw[1])<<8 | int32(raw[2])<<16
		if value&0x800000 != 0 {
			value |= ^int32(0xffffff)
		}
		return float64(value) / 8388608
	case 32:
		return float64(int32(binary.LittleEndian.Uint32(raw))) / 2147483648
	default:
		return 0
	}
}

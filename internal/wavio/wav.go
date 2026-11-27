package wavio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
)

const (
	formatPCM        = 1
	formatFloat      = 3
	formatExtensible = 0xfffe
)

// Info describes the decoded WAV container and the selected channel layout.
type Info struct {
	SampleRate int
	Channels   int
	Bits       int
	Codec      string
}

// ReadMono decodes a PCM or IEEE-float WAV file. channel < 0 downmixes all
// channels; otherwise channel is zero-based.
func ReadMono(path string, channel int) ([]float64, Info, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, Info{}, err
	}
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, Info{}, errors.New("not a RIFF/WAVE file")
	}

	var (
		fmtChunk  []byte
		dataChunk []byte
	)
	for off := 12; off+8 <= len(raw); {
		id := string(raw[off : off+4])
		size := int(binary.LittleEndian.Uint32(raw[off+4 : off+8]))
		start := off + 8
		end := start + size
		if size < 0 || end < start || end > len(raw) {
			return nil, Info{}, fmt.Errorf("invalid %q chunk size %d", id, size)
		}
		switch id {
		case "fmt ":
			fmtChunk = raw[start:end]
		case "data":
			dataChunk = raw[start:end]
		}
		off = end + (size & 1)
	}
	if len(fmtChunk) < 16 {
		return nil, Info{}, errors.New("missing or short fmt chunk")
	}
	if dataChunk == nil {
		return nil, Info{}, errors.New("missing data chunk")
	}

	tag := binary.LittleEndian.Uint16(fmtChunk[0:2])
	channels := int(binary.LittleEndian.Uint16(fmtChunk[2:4]))
	rate := int(binary.LittleEndian.Uint32(fmtChunk[4:8]))
	blockAlign := int(binary.LittleEndian.Uint16(fmtChunk[12:14]))
	bits := int(binary.LittleEndian.Uint16(fmtChunk[14:16]))
	if tag == formatExtensible {
		if len(fmtChunk) < 26 {
			return nil, Info{}, errors.New("short WAVE_FORMAT_EXTENSIBLE fmt chunk")
		}
		tag = binary.LittleEndian.Uint16(fmtChunk[24:26])
	}
	if channels <= 0 || rate <= 0 || bits <= 0 {
		return nil, Info{}, errors.New("invalid WAV format fields")
	}
	if blockAlign != channels*bits/8 {
		return nil, Info{}, fmt.Errorf("unsupported block alignment %d for %d channels x %d bits", blockAlign, channels, bits)
	}
	if len(dataChunk)%blockAlign != 0 {
		return nil, Info{}, errors.New("data chunk is not an integral number of frames")
	}

	var codec string
	switch tag {
	case formatPCM:
		codec = "PCM"
		if bits != 8 && bits != 16 && bits != 24 && bits != 32 {
			return nil, Info{}, fmt.Errorf("unsupported PCM bit depth %d", bits)
		}
	case formatFloat:
		codec = "IEEE float"
		if bits != 32 && bits != 64 {
			return nil, Info{}, fmt.Errorf("unsupported float bit depth %d", bits)
		}
	default:
		return nil, Info{}, fmt.Errorf("unsupported WAV format tag 0x%04x", tag)
	}
	if channel >= channels {
		return nil, Info{}, fmt.Errorf("channel %d out of range (file has %d)", channel, channels)
	}

	frames := len(dataChunk) / blockAlign
	out := make([]float64, frames)
	scale := 1.0 / float64(channels)
	for frame := 0; frame < frames; frame++ {
		base := frame * blockAlign
		var sum float64
		for ch := 0; ch < channels; ch++ {
			if channel >= 0 && ch != channel {
				continue
			}
			v := decodeSample(dataChunk[base+ch*bits/8:], tag, bits)
			sum += v
			if channel >= 0 {
				break
			}
		}
		if channel >= 0 {
			out[frame] = sum
		} else {
			out[frame] = sum * scale
		}
	}
	return out, Info{SampleRate: rate, Channels: channels, Bits: bits, Codec: codec}, nil
}

func decodeSample(b []byte, tag uint16, bits int) float64 {
	if tag == formatFloat {
		if bits == 32 {
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[:4])))
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(b[:8]))
	}
	switch bits {
	case 8:
		return (float64(b[0]) - 128.0) / 128.0
	case 16:
		return float64(int16(binary.LittleEndian.Uint16(b[:2]))) / 32768.0
	case 24:
		u := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
		if u&0x800000 != 0 {
			u |= 0xff000000
		}
		return float64(int32(u)) / 8388608.0
	case 32:
		return float64(int32(binary.LittleEndian.Uint32(b[:4]))) / 2147483648.0
	default:
		return 0
	}
}

// WritePCM16Mono writes a mono 16-bit PCM WAV. It is used by tests and by
// callers that need to materialize a synthetic diagnostic signal.
func WritePCM16Mono(path string, samples []float64, rate int) error {
	const bits = 16
	dataSize := len(samples) * 2
	out := make([]byte, 44+dataSize)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+dataSize))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], formatPCM)
	binary.LittleEndian.PutUint16(out[22:24], 1)
	binary.LittleEndian.PutUint32(out[24:28], uint32(rate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(rate*2))
	binary.LittleEndian.PutUint16(out[32:34], 2)
	binary.LittleEndian.PutUint16(out[34:36], bits)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataSize))
	for i, v := range samples {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		s := int16(math.Round(v * 32767))
		binary.LittleEndian.PutUint16(out[44+2*i:], uint16(s))
	}
	return os.WriteFile(path, out, 0o644)
}

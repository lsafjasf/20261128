package ingest

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/lsafjasf/20261128/internal/ltc"
)

func binaryFrames(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("binary input is empty")
	}
	if allBitBytes(data) {
		width := 64
		if len(data)%80 == 0 {
			width = 80
		} else if len(data)%64 != 0 {
			return nil, fmt.Errorf("0/1 byte stream has %d bytes; want a multiple of 64 or 80 bits", len(data))
		}
		groups := make([][]byte, 0, len(data)/width)
		for i := 0; i < len(data); i += width {
			groups = append(groups, packBits(data[i:i+width]))
		}
		return groups, nil
	}
	width := 0
	switch {
	case len(data)%10 == 0:
		width = 10
	case len(data)%8 == 0:
		width = 8
	default:
		return nil, fmt.Errorf("binary input length %d is not a multiple of 8 or 10 bytes", len(data))
	}
	groups := make([][]byte, 0, len(data)/width)
	for i := 0; i < len(data); i += width {
		groups = append(groups, data[i:i+width])
	}
	return groups, nil
}

func allBitBytes(data []byte) bool {
	for _, b := range data {
		if b != 0 && b != 1 {
			return false
		}
	}
	return true
}

func packBits(bits []byte) []byte {
	out := make([]byte, len(bits)/8)
	for i, b := range bits {
		if b == 1 {
			out[i/8] |= 1 << (7 - i%8)
		}
	}
	return out
}

func decodeBinarySet(groups [][]byte, fps int, order ltc.BitOrder) ([]ltc.Frame, error) {
	frames := make([]ltc.Frame, 0, len(groups))
	for i, group := range groups {
		f, err := ltc.DecodeHexFrame(hex.EncodeToString(group), fps, order)
		if err != nil {
			return nil, fmt.Errorf("binary frame %d: %w", i, err)
		}
		frames = append(frames, f)
	}
	return frames, nil
}

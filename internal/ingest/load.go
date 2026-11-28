package ingest

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lsafjasf/20261128/internal/ltc"
)

type Format string

const (
	FormatAuto   Format = "auto"
	FormatText   Format = "text"
	FormatBinary Format = "binary"
)

type Options struct {
	NominalFPS int
	BitOrder   ltc.BitOrder
	Format     Format
}

type Result struct {
	Frames      []ltc.Frame
	NominalFPS  int
	BitOrder    ltc.BitOrder
	Format      Format
	InferredFPS bool
}

func ReadFile(path string, opts Options) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	return ParseBytes(data, opts)
}

func ParseBytes(data []byte, opts Options) (Result, error) {
	if opts.Format == "" {
		opts.Format = FormatAuto
	}
	if opts.BitOrder == "" {
		opts.BitOrder = ltc.BitOrderAuto
	}
	if opts.Format == FormatAuto {
		opts.Format = detectFormat(data)
	}
	switch opts.Format {
	case FormatText:
		tokens, err := textTokens(data)
		if err != nil {
			return Result{}, err
		}
		return parseTokens(tokens, opts)
	case FormatBinary:
		return parseBinaryBytes(data, opts)
	default:
		return Result{}, fmt.Errorf("unknown input format %q", opts.Format)
	}
}

func detectFormat(data []byte) Format {
	trimmed := strings.TrimSpace(string(data))
	for _, r := range trimmed {
		if strings.ContainsRune("0123456789abcdefABCDEFxX:_-# \t\r\n", r) {
			continue
		}
		return FormatBinary
	}
	if len(trimmed) == 0 {
		return FormatText
	}
	return FormatText
}

func parseTokens(tokens []string, opts Options) (Result, error) {
	if len(tokens) == 0 {
		return Result{}, errors.New("no LTC frames found in text input")
	}
	rates := candidateRates(opts.NominalFPS)
	best := Result{}
	bestScore := -1 << 30
	for _, fps := range rates {
		frames, inferredOrder, err := decodeTokenSet(tokens, fps, opts.BitOrder)
		if err != nil {
			continue
		}
		score := scoreFrames(frames)
		if score > bestScore {
			bestScore = score
			best = Result{Frames: frames, NominalFPS: fps, BitOrder: inferredOrder,
				Format: FormatText, InferredFPS: opts.NominalFPS == 0}
		}
	}
	if bestScore == -1<<30 {
		return Result{}, errors.New("text input does not contain valid 64/80-bit or 16/20-hex-digit LTC frames")
	}
	if opts.NominalFPS != 0 {
		best.InferredFPS = false
	}
	return best, nil
}

func parseBinaryBytes(data []byte, opts Options) (Result, error) {
	rates := candidateRates(opts.NominalFPS)
	orders := []ltc.BitOrder{opts.BitOrder}
	if opts.BitOrder == ltc.BitOrderAuto || opts.BitOrder == "" {
		orders = []ltc.BitOrder{ltc.BitOrderMSB, ltc.BitOrderLSB}
	}
	best := Result{}
	bestScore := -1 << 30
	consider := func(frames []ltc.Frame, fps int, order ltc.BitOrder) {
		if len(frames) == 0 {
			return
		}
		score := scoreFrames(frames)
		if score > bestScore {
			bestScore = score
			best = Result{Frames: frames, NominalFPS: fps, BitOrder: order,
				Format: FormatBinary, InferredFPS: opts.NominalFPS == 0}
		}
	}
	if !allBitBytes(data) {
		for _, fps := range rates {
			for _, order := range orders {
				bits := bytesToBitString(data, order)
				frames := ltc.ExtractBitStreamFrames(bits, fps)
				consider(frames, fps, order)
			}
		}
	}
	if groups, err := binaryFrames(data); err == nil {
		for _, fps := range rates {
			for _, order := range orders {
				frames, decodeErr := decodeBinarySet(groups, fps, order)
				if decodeErr == nil {
					consider(frames, fps, order)
				}
			}
		}
	}
	if bestScore == -1<<30 {
		return Result{}, errors.New("binary input does not contain decodable LTC frames or a complete sync word")
	}
	if opts.NominalFPS != 0 {
		best.InferredFPS = false
	}
	if allBitBytes(data) {
		best.BitOrder = ""
	}
	return best, nil
}

func bytesToBitString(raw []byte, order ltc.BitOrder) string {
	b := make([]byte, 0, len(raw)*8)
	for _, v := range raw {
		for i := 0; i < 8; i++ {
			bit := (v >> (7 - i)) & 1
			if order == ltc.BitOrderLSB {
				bit = (v >> i) & 1
			}
			if bit == 1 {
				b = append(b, '1')
			} else {
				b = append(b, '0')
			}
		}
	}
	return string(b)
}

func candidateRates(fps int) []int {
	if fps != 0 {
		return []int{fps}
	}
	return []int{30, 25, 24}
}

func scoreFrames(frames []ltc.Frame) int {
	score := 0
	for _, f := range frames {
		if f.SyncValid || f.InfoBitsOnly {
			score += 100
		}
		if len(f.Warnings) == 0 {
			score += 20
		} else {
			score -= 10 * len(f.Warnings)
		}
		switch f.Date.Status {
		case ltc.DateDetermined:
			score += 30
		case ltc.DateCenturyAmbiguous:
			score += 10
		case ltc.DateInvalid:
			score -= 10
		}
		if f.DropFrame {
			score += 5
		}
	}
	return score
}

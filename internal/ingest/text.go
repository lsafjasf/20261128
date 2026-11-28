package ingest

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lsafjasf/20261128/internal/ltc"
)

func textTokens(data []byte) ([]string, error) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	if len(cleaned) == 0 {
		return nil, errors.New("text input is empty")
	}

	var tokens []string
	allLines := true
	for _, line := range cleaned {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		for _, field := range fields {
			if !validToken(field) {
				allLines = false
				break
			}
		}
		if !allLines {
			break
		}
		tokens = append(tokens, fields...)
	}
	if allLines && len(tokens) > 0 {
		return tokens, nil
	}

	joined := strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "", "_", "", ":", "").Replace(strings.Join(cleaned, ""))
	if isBitString(joined) {
		if words := ltc.SplitBitStream(joined); len(words) > 0 {
			return words, nil
		}
	}
	if isBitString(joined) && (len(joined)%64 == 0 || len(joined)%80 == 0) {
		size := 64
		if len(joined)%80 == 0 {
			size = 80
		}
		for i := 0; i < len(joined); i += size {
			tokens = append(tokens, joined[i:i+size])
		}
		return tokens, nil
	}
	if isHexString(joined) && (len(joined)%16 == 0 || len(joined)%20 == 0) {
		size := 16
		if len(joined)%20 == 0 {
			size = 20
		}
		for i := 0; i < len(joined); i += size {
			tokens = append(tokens, joined[i:i+size])
		}
		return tokens, nil
	}
	return nil, fmt.Errorf("could not split text input into LTC frames")
}

func validToken(token string) bool {
	if isBitString(token) {
		n := len(strings.NewReplacer(" ", "", "\t", "", "_", "", ":", "").Replace(token))
		return n == 64 || n == 80
	}
	if isHexString(token) {
		n := len(strings.NewReplacer(" ", "", "\t", "", "_", "", ":", "").Replace(token))
		n -= strings.Count(token, "0x") * 2
		n -= strings.Count(token, "0X") * 2
		return n == 16 || n == 20
	}
	return false
}

func isBitString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch r {
		case '0', '1', ' ', '\t', '_', ':':
		default:
			return false
		}
	}
	return true
}

func isHexString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch r {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
			'a', 'b', 'c', 'd', 'e', 'f', 'A', 'B', 'C', 'D', 'E', 'F',
			'x', 'X', ' ', '\t', '_', ':':
		default:
			return false
		}
	}
	return true
}

func decodeTokenSet(tokens []string, fps int, order ltc.BitOrder) ([]ltc.Frame, ltc.BitOrder, error) {
	orders := []ltc.BitOrder{order}
	if order == ltc.BitOrderAuto || order == "" {
		orders = []ltc.BitOrder{ltc.BitOrderMSB, ltc.BitOrderLSB}
	}
	bestScore := -1 << 30
	var best []ltc.Frame
	var bestOrder ltc.BitOrder
	hasHex := false
	for _, token := range tokens {
		if !isBitString(token) {
			hasHex = true
			break
		}
	}
	for _, candidate := range orders {
		frames := make([]ltc.Frame, 0, len(tokens))
		ok := true
		for _, token := range tokens {
			var f ltc.Frame
			var err error
			if isBitString(token) {
				f, err = ltc.ParseBitString(token, fps)
			} else {
				f, err = ltc.DecodeHexFrame(token, fps, candidate)
			}
			if err != nil {
				ok = false
				break
			}
			frames = append(frames, f)
		}
		if !ok {
			continue
		}
		score := scoreFrames(frames)
		if score > bestScore {
			bestScore = score
			best = frames
			bestOrder = candidate
		}
	}
	if best == nil {
		return nil, "", errors.New("token set is not decodable")
	}
	if !hasHex {
		bestOrder = ""
	}
	return best, bestOrder, nil
}

package ltc

// ExtractBitStreamFrames locates complete 80-bit LTC words by their sync word.
// It tolerates leading/trailing bits, so a capture does not need to begin on a
// frame boundary. The bit order must already be canonical (bit 0 first).
func ExtractBitStreamFrames(bits string, nominalFPS int) []Frame {
	words := SplitBitStream(bits)
	frames := make([]Frame, 0, len(words))
	for _, word := range words {
		frames = append(frames, DecodeBits(word, nominalFPS, false))
	}
	return frames
}

func SplitBitStream(bits string) []string {
	var words []string
	lastStart := -1
	for search := 0; search+len(SyncBits) <= len(bits); {
		rel := indexBits(bits[search:], SyncBits)
		if rel < 0 {
			break
		}
		syncPos := search + rel
		start := syncPos - 64
		if start >= 0 && start != lastStart {
			words = append(words, bits[start:start+80])
			lastStart = start
		}
		search = syncPos + 1
	}
	return words
}

func indexBits(haystack, needle string) int {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return -1
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

package wavpack

import (
	"fmt"
	"path/filepath"
	"strings"
)

const maxSampleIndex = uint64(1)<<40 - 1

func inferredCorrectionPath(mainPath string) string {
	ext := filepath.Ext(mainPath)
	switch {
	case strings.EqualFold(ext, ".wv"):
		return strings.TrimSuffix(mainPath, ext) + ".wvc"
	case ext == "":
		return mainPath + ".wvc"
	default:
		return strings.TrimSuffix(mainPath, ext) + ext + "c"
	}
}

// CorrectionPathFor returns the conventional .wvc path for a .wv file.
func CorrectionPathFor(mainPath string) string {
	return inferredCorrectionPath(mainPath)
}

func compatibleFlags(main, correction Block) bool {
	const sequenceFlags = flagInitial | flagFinal
	return main.Flags&^sequenceFlags == correction.Flags&^sequenceFlags
}

func correctionProblem(block Block) string {
	if block.MetadataError != "" {
		return "metadata: " + block.MetadataError
	}
	if block.Checksum.Present && !block.Checksum.Valid {
		if block.Checksum.Error != "" {
			return block.Checksum.Error
		}
		return "block checksum mismatch"
	}
	return "block did not pass structural validation"
}

func totalSamplesFor(blocks []Block) int64 {
	for _, block := range blocks {
		if !block.Audio() {
			continue
		}
		if total := block.TotalSamples(); total >= 0 {
			return total
		}
	}
	var lastEnd int64
	for _, block := range blocks {
		if !block.Audio() {
			continue
		}
		end := block.BlockIndex() + uint64(block.BlockSamples)
		if int64(end) > lastEnd {
			lastEnd = int64(end)
		}
	}
	if lastEnd != 0 {
		return lastEnd
	}
	return -1
}

func checkMainContinuity(blocks []Block, analysis *Analysis) {
	var haveSegment bool
	var segmentStart uint64
	var segmentSamples uint32
	var expectedStart uint64

	for _, block := range blocks {
		if !block.Audio() {
			continue
		}
		index := block.BlockIndex()
		if block.Initial() {
			if haveSegment && index != expectedStart {
				analysis.MainIssues = append(analysis.MainIssues,
					fmt.Sprintf("main block_index discontinuity: got %d, want %d", index, expectedStart))
				analysis.Complete = false
			}
			segmentStart = index
			segmentSamples = block.BlockSamples
			expectedStart = index + uint64(block.BlockSamples)
			haveSegment = true
		} else {
			if !haveSegment {
				analysis.MainIssues = append(analysis.MainIssues,
					fmt.Sprintf("block at offset %d is not marked INITIAL and has no preceding segment", block.FileOffset))
				analysis.Complete = false
			} else if index != segmentStart || block.BlockSamples != segmentSamples {
				analysis.MainIssues = append(analysis.MainIssues,
					fmt.Sprintf("multichannel block mismatch at offset %d: index/samples %d/%d, want %d/%d",
						block.FileOffset, index, block.BlockSamples, segmentStart, segmentSamples))
				analysis.Complete = false
			}
		}
		if index > maxSampleIndex || uint64(block.BlockSamples) > maxSampleIndex-index {
			analysis.MainIssues = append(analysis.MainIssues,
				fmt.Sprintf("sample range overflows 40-bit block index at block %d", index))
			analysis.Complete = false
		}
	}
}

func uniqueUint64(values []uint64) []uint64 {
	if len(values) < 2 {
		return values
	}
	seen := make(map[uint64]struct{}, len(values))
	out := values[:0]
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := values[:0]
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

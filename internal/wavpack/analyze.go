package wavpack

import (
	"fmt"
	"sort"
)

const (
	StatusMatched      = "matched"
	StatusNotRequired  = "not_required"
	StatusMissing      = "missing_correction_block"
	StatusCorrupt      = "correction_block_unusable"
	StatusIncompatible = "correction_incompatible"
	StatusIncomplete   = "correction_data_incomplete"
)

// Dependency records how one lossy main block relates to a correction block.
// CRCMatch is true when the main (lossy) header CRC equals the correction
// (lossless) header CRC. It is evidence that no residual was discarded, but it
// is not an decoded-audio CRC check.
type Dependency struct {
	BlockIndex        uint64 `json:"block_index"`
	BlockSamples      uint32 `json:"block_samples"`
	MainOffset        int64  `json:"main_offset"`
	MainCRC           uint32 `json:"main_crc"`
	CorrectionOffset  int64  `json:"correction_offset,omitempty"`
	CorrectionCRC     uint32 `json:"correction_crc,omitempty"`
	CRCMatch          bool   `json:"crc_match"`
	CorrectionPresent bool   `json:"correction_present"`
	Status            string `json:"status"`
	Detail            string `json:"detail,omitempty"`
}

// MissingRange is a half-open sample interval [Start, End) for which no usable
// correction dependency exists. Samples must not be invented for this range.
type MissingRange struct {
	Start        uint64   `json:"start"`
	End          uint64   `json:"end_exclusive"`
	Samples      uint64   `json:"samples"`
	BlockIndices []uint64 `json:"block_indices"`
	Reasons      []string `json:"reasons"`
	StartSeconds *float64 `json:"start_seconds,omitempty"`
	EndSeconds   *float64 `json:"end_seconds,omitempty"`
}

// Analysis is the complete dependency result for a .wv file and an optional
// .wvc file.
type Analysis struct {
	MainPath             string         `json:"main_path"`
	CorrectionPath       string         `json:"correction_path,omitempty"`
	CorrectionFileExists bool           `json:"correction_file_exists"`
	SampleRate           uint32         `json:"sample_rate,omitempty"`
	TotalSamples         int64          `json:"total_samples"`
	HybridBlocks         int            `json:"hybrid_blocks"`
	CorrectionBlocks     int            `json:"correction_blocks"`
	MatchedBlocks        int            `json:"matched_blocks"`
	Complete             bool           `json:"lossless_dependencies_complete"`
	Dependencies         []Dependency   `json:"dependencies"`
	MissingRanges        []MissingRange `json:"missing_ranges"`
	MainIssues           []string       `json:"main_issues,omitempty"`
	CorrectionIssues     []string       `json:"correction_issues,omitempty"`
	UnusableBlocks       int            `json:"unusable_correction_blocks"`
}

type blockKey struct {
	index   uint64
	samples uint32
}

// Analyze correlates hybrid main blocks with correction blocks. A missing or
// unusable matching block is reported as a missing sample range; no audio is
// synthesized.
func Analyze(main, correction *ScanResult) *Analysis {
	analysis := &Analysis{
		MainPath:       main.Path,
		SampleRate:     sampleRateFor(main.Blocks),
		MainIssues:     append([]string(nil), main.Issues...),
		Complete:       true,
		MissingRanges:  []MissingRange{},
		Dependencies:   []Dependency{},
		UnusableBlocks: 0,
	}
	if correction != nil {
		analysis.CorrectionPath = correction.Path
		analysis.CorrectionFileExists = true
		analysis.CorrectionBlocks = len(correction.Blocks)
		analysis.CorrectionIssues = append([]string(nil), correction.Issues...)
	} else {
		analysis.CorrectionPath = inferredCorrectionPath(main.Path)
	}

	analysis.TotalSamples = totalSamplesFor(main.Blocks)
	checkMainContinuity(main.Blocks, analysis)

	var corrBlocks []Block
	if correction != nil {
		corrBlocks = correction.Blocks
	}
	mainGroups := make(map[blockKey][]Block)
	corrGroups := make(map[blockKey][]Block)

	for _, block := range main.Blocks {
		if !block.Audio() || !block.Hybrid() {
			continue
		}
		analysis.HybridBlocks++
		if !block.Valid() {
			analysis.MainIssues = append(analysis.MainIssues,
				fmt.Sprintf("hybrid block at offset %d is structurally invalid", block.FileOffset))
			analysis.Complete = false
			continue
		}
		key := blockKey{index: block.BlockIndex(), samples: block.BlockSamples}
		mainGroups[key] = append(mainGroups[key], block)
	}
	for _, block := range corrBlocks {
		key := blockKey{index: block.BlockIndex(), samples: block.BlockSamples}
		corrGroups[key] = append(corrGroups[key], block)
	}

	for key, mains := range mainGroups {
		corrs := corrGroups[key]
		for i, want := range mains {
			dep := Dependency{
				BlockIndex:   key.index,
				BlockSamples: key.samples,
				MainOffset:   want.FileOffset,
				MainCRC:      want.CRC,
			}
			if i >= len(corrs) {
				dep.Status = StatusMissing
				dep.Detail = "no correction block with matching block_index and block_samples"
				analysis.Dependencies = append(analysis.Dependencies, dep)
				analysis.addMissing(want, StatusMissing)
				continue
			}
			have := corrs[i]
			dep.CorrectionPresent = true
			dep.CorrectionOffset = have.FileOffset
			dep.CorrectionCRC = have.CRC
			dep.CRCMatch = want.CRC == have.CRC

			switch {
			case !have.Valid():
				dep.Status = StatusCorrupt
				dep.Detail = correctionProblem(have)
				analysis.UnusableBlocks++
				analysis.addMissing(want, StatusCorrupt)
			case !compatibleFlags(want, have):
				dep.Status = StatusIncompatible
				dep.Detail = fmt.Sprintf("header flags 0x%08x != 0x%08x", want.Flags, have.Flags)
				analysis.UnusableBlocks++
				analysis.addMissing(want, StatusIncompatible)
			case have.CorrectionData():
				dep.Status = StatusMatched
				dep.Detail = "correction residual present"
				analysis.MatchedBlocks++
			case dep.CRCMatch:
				dep.Status = StatusNotRequired
				dep.Detail = "header CRCs match and correction block contains no residual"
				analysis.MatchedBlocks++
			default:
				dep.Status = StatusIncomplete
				dep.Detail = "lossy/lossless CRCs differ but correction block has no WVC/WVX data"
				analysis.UnusableBlocks++
				analysis.addMissing(want, StatusIncomplete)
			}
			analysis.Dependencies = append(analysis.Dependencies, dep)
		}

		for i := len(mains); i < len(corrs); i++ {
			analysis.CorrectionIssues = append(analysis.CorrectionIssues,
				fmt.Sprintf("orphan correction block at offset %d (index %d, samples %d)",
					corrs[i].FileOffset, key.index, key.samples))
		}
	}

	for key, corrs := range corrGroups {
		if _, ok := mainGroups[key]; ok {
			continue
		}
		for _, block := range corrs {
			analysis.CorrectionIssues = append(analysis.CorrectionIssues,
				fmt.Sprintf("orphan correction block at offset %d (index %d, samples %d)",
					block.FileOffset, key.index, key.samples))
		}
	}

	sort.Slice(analysis.Dependencies, func(i, j int) bool {
		if analysis.Dependencies[i].BlockIndex != analysis.Dependencies[j].BlockIndex {
			return analysis.Dependencies[i].BlockIndex < analysis.Dependencies[j].BlockIndex
		}
		return analysis.Dependencies[i].MainOffset < analysis.Dependencies[j].MainOffset
	})
	analysis.addRangeTimes()
	analysis.Complete = analysis.Complete && len(analysis.MissingRanges) == 0
	return analysis
}

func (a *Analysis) addMissing(block Block, reason string) {
	end := block.BlockIndex() + uint64(block.BlockSamples)
	a.MissingRanges = append(a.MissingRanges, MissingRange{
		Start:        block.BlockIndex(),
		End:          end,
		Samples:      uint64(block.BlockSamples),
		BlockIndices: []uint64{block.BlockIndex()},
		Reasons:      []string{reason},
	})
	a.Complete = false
}

func (a *Analysis) addRangeTimes() {
	sort.Slice(a.MissingRanges, func(i, j int) bool {
		return a.MissingRanges[i].Start < a.MissingRanges[j].Start
	})
	var merged []MissingRange
	for _, r := range a.MissingRanges {
		if len(merged) == 0 || r.Start > merged[len(merged)-1].End {
			merged = append(merged, r)
			continue
		}
		last := &merged[len(merged)-1]
		if r.End > last.End {
			last.End = r.End
			last.Samples = last.End - last.Start
		}
		last.BlockIndices = append(last.BlockIndices, r.BlockIndices...)
		last.Reasons = append(last.Reasons, r.Reasons...)
	}
	for i := range merged {
		merged[i].BlockIndices = uniqueUint64(merged[i].BlockIndices)
		merged[i].Reasons = uniqueStrings(merged[i].Reasons)
		if a.SampleRate != 0 {
			start := float64(merged[i].Start) / float64(a.SampleRate)
			end := float64(merged[i].End) / float64(a.SampleRate)
			merged[i].StartSeconds = &start
			merged[i].EndSeconds = &end
		}
	}
	a.MissingRanges = merged
}

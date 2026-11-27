package main

import (
	"fmt"
	"os"
	"strings"

	"wvcheck/internal/wavpack"
)

func printText(result *wavpack.Analysis, verbose bool) {
	fmt.Println("WavPack correction dependency check")
	fmt.Printf("main:       %s\n", result.MainPath)
	if result.CorrectionFileExists {
		fmt.Printf("correction: %s\n", result.CorrectionPath)
	} else {
		fmt.Printf("correction: %s (absent)\n", result.CorrectionPath)
	}
	if result.SampleRate != 0 {
		fmt.Printf("sample rate: %d Hz\n", result.SampleRate)
	}
	fmt.Printf("total samples: %d\n", result.TotalSamples)
	fmt.Printf("hybrid blocks: %d, correction blocks: %d, usable matches: %d\n",
		result.HybridBlocks, result.CorrectionBlocks, result.MatchedBlocks)

	if result.Complete {
		fmt.Println("status: complete - every hybrid block has a usable correction dependency")
	} else {
		fmt.Println("status: INCOMPLETE - lossless restoration is not possible for the ranges below")
	}

	for _, issue := range result.MainIssues {
		fmt.Printf("main issue: %s\n", issue)
	}
	for _, issue := range result.CorrectionIssues {
		fmt.Printf("correction issue: %s\n", issue)
	}

	if len(result.MissingRanges) > 0 {
		fmt.Println("missing correction sample ranges (half-open [start, end)):")
		for _, r := range result.MissingRanges {
			fmt.Printf("  [%d, %d)  %d samples", r.Start, r.End, r.Samples)
			if r.StartSeconds != nil && r.EndSeconds != nil {
				fmt.Printf("  %.6f-%.6fs", *r.StartSeconds, *r.EndSeconds)
			}
			fmt.Println()
			fmt.Printf("    blocks: %s\n", joinUint64(r.BlockIndices))
			fmt.Printf("    reasons: %s\n", strings.Join(r.Reasons, ", "))
		}
	}

	if verbose {
		fmt.Println("block dependencies:")
		for _, dep := range result.Dependencies {
			crcState := "crc differs"
			if dep.CRCMatch {
				crcState = "crc matches"
			}
			fmt.Printf("  index=%d samples=%d main_crc=%08x correction_crc=%08x %s status=%s",
				dep.BlockIndex, dep.BlockSamples, dep.MainCRC, dep.CorrectionCRC, crcState, dep.Status)
			if dep.Detail != "" {
				fmt.Printf(" (%s)", dep.Detail)
			}
			fmt.Println()
		}
	}
	if !result.Complete {
		fmt.Fprintln(os.Stderr, "note: no missing samples were generated or interpolated")
	}
}

func joinUint64(values []uint64) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprintf("%d", value)
	}
	return strings.Join(parts, ", ")
}

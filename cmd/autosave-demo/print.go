package main

import (
	"fmt"

	"editorautosave/internal/demo"
	"editorautosave/recovery"
)

func printDemoSummary(s *demo.Summary) {
	fmt.Println("autosave recovery demo")
	fmt.Printf("workspace:                %s\n", s.Root)
	fmt.Printf("disk project (stale):     %s seq=%d\n", s.DiskProjectPath, s.DiskSeqBeforeCrash)
	fmt.Printf("autosave head pre-crash:  seq=%d, last successful write seq=%d\n",
		s.HeadSeqBeforeCrash, s.LastSuccessSeq)
	fmt.Printf("first recovery output:    %s\n", s.RecoveredFirst)
	fmt.Printf("corrupt snapshot id:      %s\n", s.BreakSnapshotID)
	fmt.Printf("fallback recovery output: %s seq=%d\n", s.RecoveredFallback, s.FallbackSeq)
	fmt.Printf("original project kept:    %t\n", s.OriginalUntouched)
	for _, warning := range s.MissingMedia {
		fmt.Printf("media warning:            %s\n", warning)
	}
}

func printReport(report *recovery.Report) {
	fmt.Printf("project:            %s\n", report.ProjectID)
	fmt.Printf("disk project:       %s (seq %d)\n", report.DiskProjectPath, report.DiskProjectSeq)
	fmt.Printf("last success write: seq %d at %s\n", report.LastSuccessSeq, report.LastSuccessAt)
	if report.Break != nil {
		fmt.Printf("chain break:        snapshot %s (seq %d): %s\n",
			report.Break.SnapshotID, report.Break.Seq, report.Break.Reason)
	}
	fmt.Println("candidates (newest first):")
	for _, cand := range report.Candidates {
		status := "reachable"
		if !cand.Reachable {
			status = "UNREACHABLE: " + cand.Error
		}
		fmt.Printf("  - %s seq=%-4d kind=%-11s %s created=%s\n",
			cand.ID, cand.Seq, cand.Kind, status, cand.CreatedAt)
		for _, missing := range cand.MissingMedia {
			fmt.Printf("      media warning: %s\n", missing)
		}
	}
	if report.Best != nil {
		newer := ""
		if report.HasNewerCandidate() {
			newer = " (newer than disk project)"
		}
		fmt.Printf("recommended:        %s seq=%d%s\n", report.Best.ID, report.Best.Seq, newer)
	} else {
		fmt.Println("recommended:        none; keep the original on-disk project")
	}
}

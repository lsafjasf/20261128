package main

import (
	"fmt"

	"lto-manifest/internal/budget"
	"lto-manifest/internal/manifest"
)

func printScanSummary(m *manifest.Manifest, outPath string) {
	var media, sidecar, other int64
	for _, e := range m.Entries {
		switch e.Class {
		case "media":
			media++
		case "sidecar":
			sidecar++
		default:
			other++
		}
	}
	fmt.Printf("manifest written: %s\n", outPath)
	fmt.Printf("entries: %d (media %d, sidecar %d, other %d), total %s\n",
		m.EntryCount, media, sidecar, other, budget.Format(m.TotalSize))
	fmt.Printf("sidecar dependencies: %d\n", len(m.Relationships))
	fmt.Printf("duplicate groups: %d\n", len(m.Duplicates))
	for _, d := range m.Duplicates {
		fmt.Printf("  %s x%d  waste %s\n", shortHash(d.SHA256), len(d.Paths), budget.Format(d.Wasted))
	}
	for _, issue := range m.Issues {
		fmt.Printf("issue [%s] %s: %s\n", issue.Code, issue.Path, issue.Detail)
	}
	if m.Budget != nil {
		status := "within budget"
		if m.Budget.Over {
			status = "OVER BUDGET"
		}
		fmt.Printf("budget: %s / %s (%.1f%%) unique %s, reclaimable %s - %s\n",
			budget.Format(m.Budget.TotalSize), budget.Format(m.Budget.Limit),
			m.Budget.Ratio*100, budget.Format(m.Budget.UniqueSize),
			budget.Format(m.Budget.Reclaimable), status)
	}
}

func printVerifySummary(r *manifest.VerifyResult) {
	status := "OK"
	if !r.OK {
		status = "FAILED"
	}
	mode := "re-layout tolerant"
	if r.Strict {
		mode = "strict"
	}
	fmt.Printf("verify %s (%s)\n", status, mode)
	fmt.Printf("intact: %d, rehomed: %d, missing: %d, corrupt: %d, extra: %d\n",
		len(r.Intact), len(r.Rehomed), len(r.Missing), len(r.Corrupt), len(r.Extra))
	for _, re := range r.Rehomed {
		fmt.Printf("rehomed: %s -> %s (%s)\n", re.OldPath, re.NewPath, shortHash(re.SHA256))
	}
	for _, c := range r.Corrupt {
		fmt.Printf("corrupt: %s expected %s got %s\n", c.Path, shortHash(c.ExpectedSHA), shortHash(c.ActualSHA))
	}
	for _, p := range r.Missing {
		fmt.Printf("missing: %s\n", p)
	}
	for _, p := range r.Extra {
		fmt.Printf("extra:   %s\n", p)
	}
	for _, issue := range r.ColocationIssue {
		fmt.Printf("issue [%s] %s: %s\n", issue.Code, issue.Path, issue.Detail)
	}
	for _, issue := range r.Issues {
		fmt.Printf("issue [%s] %s: %s\n", issue.Code, issue.Path, issue.Detail)
	}
}

func shortHash(h string) string {
	if len(h) < 12 {
		return h
	}
	return h[:12]
}

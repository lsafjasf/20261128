package main

import (
	"fmt"
	"strings"

	"subaudit/internal/pair"
)

var verdictLabels = map[pair.Verdict]string{
	pair.VerdictSame:     "相同",
	pair.VerdictPartial:  "部分变化",
	pair.VerdictMismatch: "完全不一致",
}

func printReport(r *pair.Report) {
	fmt.Println("字幕配对审计报告")
	fmt.Println(strings.Repeat("=", 72))
	for i, p := range r.Pairs {
		fmt.Printf("[%d] 语言=%s  判定=%s  匹配分=%d\n", i+1, p.Language,
			verdictLabels[p.Verdict], p.Score)
		fmt.Printf("    内嵌: %s (%s)\n", p.Embedded.Path, p.Embedded.Format)
		fmt.Printf("    外挂: %s (%s)\n", p.Sidecar.Path, p.Sidecar.Format)
		for _, d := range p.Dims {
			mark := "✓"
			if !d.Equal {
				mark = "✗"
			}
			fmt.Printf("      %s %-13s %s\n", mark, d.Name, d.Detail)
		}
		if len(p.Reasons) > 0 {
			fmt.Println("    差异:")
			for _, reason := range p.Reasons {
				fmt.Printf("      - %s\n", reason)
			}
		}
		fmt.Println()
	}

	if len(r.UnpairedEmbedded) > 0 || len(r.UnpairedSidecar) > 0 {
		fmt.Println("未配对条目（无法基于指纹确认对应关系，文件名不作为依据）:")
		for _, s := range r.UnpairedEmbedded {
			fmt.Printf("  - 仅内嵌存在: %s 语言=%s cue=%d\n", s.Path, s.Language, s.CueCount)
		}
		for _, s := range r.UnpairedSidecar {
			fmt.Printf("  - 仅外挂存在: %s 语言=%s cue=%d\n", s.Path, s.Language, s.CueCount)
		}
		fmt.Println()
	}

	fmt.Println("汇总:")
	fmt.Printf("  相同(SAME)            %d\n", r.Summary.Same)
	fmt.Printf("  部分变化(PARTIAL)     %d\n", r.Summary.PartialChange)
	fmt.Printf("  完全不一致(MISMATCH)  %d\n", r.Summary.Mismatch)
	fmt.Printf("  未配对(UNPAIRED)      %d\n", r.Summary.Unpaired)
}

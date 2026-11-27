package main

import (
	"fmt"
	"math"
	"strings"
)

// printTimeline 打印四路合并时间线：每行一个时间桶，
// 每路一格 "帧数 q队列深度 w写入耗时ms d丢帧"，异常格以 * 标记。
func printTimeline(grid [][]Bucket, nCh int, t0 float64, devs []Deviation) {
	devAt := map[int]map[int]bool{} // bucket -> ch -> deviated
	for _, d := range devs {
		b := int((d.TS - t0) / bucketSec)
		if devAt[b] == nil {
			devAt[b] = map[int]bool{}
		}
		devAt[b][d.Ch] = true
	}

	fmt.Println("== 四路合并时间线（每桶 200ms，格式: 出帧 q队列 w写入ms d丢帧）==")
	hdr := "  t(s)  │"
	for ch := 0; ch < nCh; ch++ {
		hdr += fmt.Sprintf("      机位%d       │", ch)
	}
	fmt.Println(hdr)
	fmt.Println(strings.Repeat("─", len(hdr)+2))

	for b := 0; b < len(grid); b++ {
		row := fmt.Sprintf(" %5.1f  │", float64(b)*bucketSec)
		for ch := 0; ch < nCh; ch++ {
			bk := grid[b][ch]
			cell := fmt.Sprintf(" %2d q%-4.1f w%5.1f d%-2d",
				bk.CamFrames, avg(bk.QSum, bk.QN), avg(bk.WrSum, bk.WrN), bk.Drops)
			if devAt[b][ch] {
				cell += "*"
			} else {
				cell += " "
			}
			row += cell + "│"
		}
		fmt.Println(row)
	}
	fmt.Println()
}

func printFrameCounts(fileFrames map[int]int, nCh int, dur float64) {
	fmt.Println("== 最终文件帧数 ==")
	expected := int(math.Round(dur * simFPS))
	for ch := 0; ch < nCh; ch++ {
		got := fileFrames[ch]
		status := "OK"
		if got < expected {
			status = fmt.Sprintf("缺 %d 帧", expected-got)
		}
		fmt.Printf("  机位%d: %d / 预期约 %d 帧  %s\n", ch, got, expected, status)
	}
	fmt.Println()
}

func printConclusion(devs []Deviation) {
	fmt.Println("== 偏离事件（按时间先后）==")
	seen := map[string]bool{}
	var firsts []Deviation
	for _, d := range devs {
		key := fmt.Sprintf("%d/%s", d.Ch, d.Sub)
		if !seen[key] {
			seen[key] = true
			firsts = append(firsts, d)
		}
	}
	for _, d := range firsts {
		fmt.Printf("  t=%5.1fs 机位%d [%-7s] %s\n", d.TS, d.Ch, d.Sub, d.Desc)
	}
	fmt.Println()
	if len(firsts) == 0 {
		fmt.Println("结论：未检测到偏离。")
		return
	}
	root := firsts[0]
	fmt.Printf("结论：首个偏离发生在 t=%.1fs，子系统 = %s（机位%d）：%s。\n",
		root.TS, root.Sub, root.Ch, root.Desc)
	fmt.Println("     后续偏离为反压沿流水线上游传播的结果，根因即首个偏离的子系统。")
}

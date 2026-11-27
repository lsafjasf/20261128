// halfframe-demo 演示 59.94fps(1001/60000) 与 29.97fps(1001/30000)
// 之间的纯整数时间基换算、半帧候选、累计误差以及十次无损往返。
package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"halfframe/timebase"
)

func main() {
	// 一组字幕入点（59.94 细帧 tick），奇偶混合以覆盖整帧与半帧。
	inPoints := []int64{240, 241, 242, 243, 244, 245, 246}

	printCandidates(inPoints)
	printCumulative(inPoints)
	printRoundTrips([]int64{241, 243, 11987})
}

func printCandidates(ticks []int64) {
	fmt.Println("== 半帧候选（细帧 59.94 -> 粗帧 29.97，误差单位：细帧） ==")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "fineTick\t精确粗帧tick\t候选\t粗帧tick\t单步误差")
	for _, fine := range ticks {
		exact, cands := timebase.Analyze(fine)
		for i, c := range cands {
			if i == 0 {
				fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%+d\n", fine, exact, c.Label, c.Tick, c.ErrorFine)
			} else {
				fmt.Fprintf(w, "\t\t%s\t%d\t%+d\n", c.Label, c.Tick, c.ErrorFine)
			}
		}
	}
	w.Flush()
	fmt.Println()
}

func printCumulative(ticks []int64) {
	fmt.Println("== 累计误差（floor 恒向下 / ceil 恒向上，单位：细帧） ==")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "fineTick\tfloor累计\tceil累计")
	var floorSum, ceilSum int64
	for _, fine := range ticks {
		floorSum += timebase.CoarseErrorFine(fine, timebase.FloorCoarse(fine))
		ceilSum += timebase.CoarseErrorFine(fine, timebase.CeilCoarse(fine))
		fmt.Fprintf(w, "%d\t%+d\t%+d\n", fine, floorSum, ceilSum)
	}
	w.Flush()
	fmt.Println()
}

func printRoundTrips(starts []int64) {
	fmt.Println("== 十次往返：无损 Stamp vs naive floor 吸附 ==")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "起始fineTick\t往返次数\t无损结果\tnaive结果")
	for _, start := range starts {
		lossless, naive := start, start
		for i := 1; i <= 10; i++ {
			lossless = timebase.RoundTrip(lossless)
			naive = timebase.NaiveSnapFloor(naive)
			fmt.Fprintf(w, "%d\t%d\t%d\t%d\n", start, i, lossless, naive)
		}
	}
	w.Flush()
}

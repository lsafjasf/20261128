package main

import (
	"fmt"
	"math"
	"sort"
)

// Bucket 是一个时间窗内单路各子系统的聚合指标。
type Bucket struct {
	CamFrames int
	QSum      float64
	QN        int
	Drops     int
	EncSum    float64
	EncN      int
	WrSum     float64
	WrN       int
}

type Deviation struct {
	TS   float64
	Ch   int
	Sub  string
	Desc string
}

const (
	bucketSec   = 0.2 // 时间线分桶宽度
	baselineSec = 1.0 // 用首个 1 秒作为健康基线
)

func analyze(path string) error {
	recs, err := LoadRecords(path)
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		return fmt.Errorf("日志为空")
	}

	t0 := math.Inf(1)
	tEnd := math.Inf(-1)
	nCh := 0
	fileFrames := map[int]int{}
	for _, r := range recs {
		if r.TS < t0 {
			t0 = r.TS
		}
		if r.TS > tEnd {
			tEnd = r.TS
		}
		if r.Ch+1 > nCh {
			nCh = r.Ch + 1
		}
		if r.Sub == "file" {
			fileFrames[r.Ch] = r.Frames
		}
	}
	nBuckets := int((tEnd-t0)/bucketSec) + 1

	grid := make([][]Bucket, nBuckets)
	for i := range grid {
		grid[i] = make([]Bucket, nCh)
	}
	for _, r := range recs {
		b := int((r.TS - t0) / bucketSec)
		if b < 0 || b >= nBuckets {
			continue
		}
		bk := &grid[b][r.Ch]
		switch r.Sub {
		case "camera":
			bk.CamFrames++
		case "capture":
			bk.QSum += float64(r.QDepth)
			bk.QN++
			if r.Drop {
				bk.Drops++
			}
		case "encode":
			bk.EncSum += r.DurMs
			bk.EncN++
		case "storage":
			bk.WrSum += r.WriteMs
			bk.WrN++
		}
	}

	devs := detectDeviations(grid, nCh, t0)
	printTimeline(grid, nCh, t0, devs)
	printFrameCounts(fileFrames, nCh, tEnd-t0)
	printConclusion(devs)
	return nil
}

func avg(sum float64, n int) float64 {
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// detectDeviations 以首秒为基线，找出每路每个子系统越过阈值的时刻。
func detectDeviations(grid [][]Bucket, nCh int, t0 float64) []Deviation {
	nBase := int(baselineSec / bucketSec)
	var devs []Deviation
	for ch := 0; ch < nCh; ch++ {
		var baseCam, baseEnc, baseWr []float64
		for b := 0; b < nBase && b < len(grid); b++ {
			bk := grid[b][ch]
			baseCam = append(baseCam, float64(bk.CamFrames))
			if bk.EncN > 0 {
				baseEnc = append(baseEnc, avg(bk.EncSum, bk.EncN))
			}
			if bk.WrN > 0 {
				baseWr = append(baseWr, avg(bk.WrSum, bk.WrN))
			}
		}
		cam0 := median(baseCam)
		enc0 := median(baseEnc)
		wr0 := median(baseWr)

		for b := nBase; b < len(grid); b++ {
			bk := grid[b][ch]
			ts := t0 + float64(b)*bucketSec
			if cam0 > 0 && float64(bk.CamFrames) < 0.7*cam0 {
				devs = append(devs, Deviation{ts, ch, "camera",
					fmt.Sprintf("相机出帧 %d -> 基线 %.0f 帧/桶", bk.CamFrames, cam0)})
			}
			if bk.Drops > 0 {
				devs = append(devs, Deviation{ts, ch, "capture",
					fmt.Sprintf("采集丢帧 %d 帧（队列满）", bk.Drops)})
			} else if bk.QN > 0 && avg(bk.QSum, bk.QN) > 0.75*queueCap {
				devs = append(devs, Deviation{ts, ch, "capture",
					fmt.Sprintf("队列深度 %.1f 超过容量 %d 的 75%%", avg(bk.QSum, bk.QN), queueCap)})
			}
			if bk.EncN > 0 && enc0 > 0 && avg(bk.EncSum, bk.EncN) > 2*enc0 {
				devs = append(devs, Deviation{ts, ch, "encode",
					fmt.Sprintf("编码耗时 %.1fms -> 基线 %.1fms", avg(bk.EncSum, bk.EncN), enc0)})
			}
			if bk.WrN > 0 && wr0 > 0 && avg(bk.WrSum, bk.WrN) > 3*wr0 {
				devs = append(devs, Deviation{ts, ch, "storage",
					fmt.Sprintf("写入耗时 %.1fms -> 基线 %.1fms", avg(bk.WrSum, bk.WrN), wr0)})
			}
		}
	}
	sort.Slice(devs, func(i, j int) bool { return devs[i].TS < devs[j].TS })
	return devs
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return s[len(s)/2]
}

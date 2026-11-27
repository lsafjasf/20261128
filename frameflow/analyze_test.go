package main

import (
	"os"
	"path/filepath"
	"testing"
)

// 端到端：仿真注入 ch2 存储故障后，分析器应判定首个偏离为 storage/机位2。
func TestFirstDeviationIsStorage(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs.jsonl")
	if err := runSim(logs, 1); err != nil {
		t.Fatal(err)
	}
	recs, err := LoadRecords(logs)
	if err != nil {
		t.Fatal(err)
	}
	t0 := recs[0].TS
	nCh := 0
	tEnd := 0.0
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
	}
	nBuckets := int((tEnd-t0)/bucketSec) + 1
	grid := make([][]Bucket, nBuckets)
	for i := range grid {
		grid[i] = make([]Bucket, nCh)
	}
	for _, r := range recs {
		b := int((r.TS - t0) / bucketSec)
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
	if len(devs) == 0 {
		t.Fatal("未检测到任何偏离")
	}
	first := devs[0]
	if first.Sub != "storage" || first.Ch != faultChannel {
		t.Fatalf("首个偏离 = %s/机位%d，期望 storage/机位%d", first.Sub, first.Ch, faultChannel)
	}
	if first.TS-t0 < faultStart-0.3 || first.TS-t0 > faultStart+0.5 {
		t.Fatalf("首个偏离时刻 %.2fs 不在故障注入点 %v 附近", first.TS-t0, faultStart)
	}
	// 编码子系统不应出现在偏离列表中（编码日志正常）
	for _, d := range devs {
		if d.Sub == "encode" {
			t.Fatalf("编码被误报为偏离: %+v", d)
		}
	}
	if os.Getenv("VERBOSE") != "" {
		for _, d := range devs[:2] {
			t.Logf("%+v", d)
		}
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
)

// 仿真参数：4 机位、30fps、10 秒。
// 故障注入：ch2 的存储写入在 t=4s 后由 ~5ms 劣化为 60~90ms（磁盘卡顿），
// 反压沿 存储→采集队列→丢帧 传播，编码器自身耗时始终正常。
const (
	simChannels  = 4
	simFPS       = 30.0
	simDuration  = 10.0 // 秒
	queueCap     = 8
	faultChannel = 2
	faultStart   = 4.0 // 秒
)

func runSim(path string, seed int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	rng := rand.New(rand.NewSource(seed))

	for ch := 0; ch < simChannels; ch++ {
		simChannel(enc, rng, ch)
	}
	fmt.Printf("已生成仿真日志: %s\n", path)
	return nil
}

func simChannel(enc *json.Encoder, rng *rand.Rand, ch int) {
	frameInterval := 1.0 / simFPS
	total := int(simDuration * simFPS)
	queue := 0
	nextFree := 0.0 // 编码+写盘流水线下一次空闲时刻
	written := 0

	for i := 0; i < total; i++ {
		// camera：帧产生时刻，带轻微时钟抖动
		ts := float64(i)*frameInterval + (rng.Float64()-0.5)*0.002
		emit(enc, Record{TS: ts, Ch: ch, Sub: "camera", Frame: i})

		// capture：流水线每消费一帧，队列减一
		if queue > 0 && nextFree <= ts {
			queue--
			nextFree = ts // 流水线从该帧到达时刻开始处理
		}
		if queue >= queueCap {
			emit(enc, Record{TS: ts, Ch: ch, Sub: "capture", Frame: i, QDepth: queue, Drop: true})
			continue
		}
		queue++
		emit(enc, Record{TS: ts, Ch: ch, Sub: "capture", Frame: i, QDepth: queue})

		// encode：耗时恒定正常（本例中编码日志无异常）
		encMs := 3.5 + rng.Float64()*1.5
		emit(enc, Record{TS: ts, Ch: ch, Sub: "encode", Frame: i, DurMs: encMs})

		// storage：写入耗时，故障通道在 faultStart 后劣化
		wrMs := 4.0 + rng.Float64()*2.0
		if ch == faultChannel && ts >= faultStart {
			wrMs = 60.0 + rng.Float64()*30.0
		}
		emit(enc, Record{TS: ts, Ch: ch, Sub: "storage", Frame: i, WriteMs: wrMs})

		nextFree = ts + (encMs+wrMs)/1000.0
		written++
	}
	emit(enc, Record{TS: simDuration, Ch: ch, Sub: "file", Frame: -1, Frames: written})
}

func emit(enc *json.Encoder, r Record) {
	if err := enc.Encode(r); err != nil {
		fmt.Fprintln(os.Stderr, "写日志失败:", err)
		os.Exit(1)
	}
}

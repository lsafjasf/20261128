package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// Record 是四路流水线上某一子系统产生的一条遥测日志（JSONL 一行）。
// Sub 取值: camera / capture / encode / storage / file
type Record struct {
	TS      float64 `json:"ts"`    // 秒，单调时钟，四路共用同一时间基准
	Ch      int     `json:"ch"`    // 机位 0..3
	Sub     string  `json:"sub"`   // 子系统
	Frame   int     `json:"frame"` // 帧序号（file 记录中为 -1）
	QDepth  int     `json:"qdepth,omitempty"`
	Drop    bool    `json:"drop,omitempty"`
	DurMs   float64 `json:"dur_ms,omitempty"`   // encode 耗时
	WriteMs float64 `json:"write_ms,omitempty"` // storage 写入耗时
	Frames  int     `json:"frames,omitempty"`   // file 记录：最终文件帧数
}

func LoadRecords(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("解析日志行失败: %w", err)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

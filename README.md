# 20261128

Repository initialized for 20261128.

## st2110 — SMPTE ST 2110-30 接收分析库 (Go)

针对 "RTP 时间与 PTP 锁状态不一致、接收端时好时坏" 的排查与防护：

- `rtp.go` — RTP 解析（CSRC/扩展头/填充跳过，版本与截断校验）。
- `audio.go` — L16/L24 格式，按 `payload / (字节/采样 × 声道数)` 计算每包采样数，
  非整帧即报错。
- `tracker.go` — 序列号分类（uint16 回绕安全）：`InOrder / Duplicate /
  Reordered / Late`，前跳累计丢包数。
- `redundancy.go` — ST 2022-7 双路合并：先到先赢、重复丢弃、单路故障无缝接管，
  分腿统计。
- `clock.go` — PTP 时钟门控：`Now()` 仅在锁定时可用，接口层面不存在
  解锁后的墙钟回退。
- `receiver.go` — 总装：ST 2110-30 时间戳连续性（步进 == 前包采样数）、
  RTP↔PTP 漂移检测（超容差即事件，提示发送端未锁同一 grandmaster）。
  PTP 失锁时进入显式降级：`DegradeMute`（静音丢弃）或
  `DegradePassThrough`（转发但标记 `Degraded`，禁用一切定时校验），
  恢复锁定后重新锚定 RTP↔PTP 映射。本地墙钟绝不悄悄接管。

```go
clk := st2110.NewPTPClock(nil) // 生产环境由 ptp4l 状态驱动 SetLocked
r, _ := st2110.NewReceiver(
    st2110.Format{Encoding: st2110.EncodingL24, Channels: 8, SampleRate: 48000},
    clk, st2110.DegradeMute)
r.OnEvent = func(e st2110.Event) { log.Printf("%v leg=%s seq=%d %s", e.Kind, e.Leg, e.Seq, e.Detail) }
if out := r.Push(st2110.LegA, datagram); out != nil && !out.Degraded {
    play(out.Packet.Payload)
}
```

测试：`go test ./...`

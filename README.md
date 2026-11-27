# 20261128

Repository initialized for 20261128.

## 半帧时间基换算（Go，纯整数）

59.94fps 素材转录为 29.97fps 时，两个时间基为分数：

- 细帧（59.94）：每 tick `1001/60000` 秒
- 粗帧（29.97）：每 tick `1001/30000` 秒，1 粗帧 = 2 细帧

通用整数换算（`timebase/timebase.go` 的 `ConvertRat`）：

```
dstTick = tick * src.Num * dst.Den / (src.Den * dst.Num)
```

结果保存为约分后的有理数 `Rat{Num, Den}`，全程不使用浮点。
偶数细帧 tick 得到整数粗帧 tick；奇数细帧 tick 得到 `k+1/2`，
即半帧，`Analyze` 返回两个候选：

- `floor`：吸附到较早粗帧，误差 `-1` 个细帧（约 -16.68ms）
- `ceil`：吸附到较晚粗帧，误差 `+1` 个细帧（约 +16.68ms）

误差口径统一为「候选时刻 − 精确时刻」，以细帧为整数单位；
`CumulativeErrorFine` 按入点序列累计。

### 为什么往返不会吸附

直接 floor/ceil 成整数粗帧 tick 会永久丢弃半帧余数（见 `NaiveSnapFloor`，
第一次往返就吸附到相邻帧）。无损做法是 `Stamp{Tick, Rem, Den}`：

- `Encode`：`Tick = fine/2`，`Rem = fine%2`，`Den = 2`
- `Decode`：`fine = Tick*2 + Rem*2/Den`

余数被显式保留，`RoundTrip` 连续十次仍还原为原始细帧 tick，
测试见 `timebase/timebase_test.go` 的 `TestLosslessRoundTripTenTimes`。

### 运行

```
go test ./...
go run ./cmd/halfframe-demo
```

整数范围：中间量为 `int64`，按约 80 年时长的 tick 数量仍不溢出，
覆盖常规字幕/时间码场景。

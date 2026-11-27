# AMR-WB 样本时间线

纯 Go 标准库实现，用于把 AMR-WB 帧头、编码模式和帧长转换为连续的样本时间线。

## 核心不变量

- AMR-WB 采样率为 16 kHz，每帧 20 ms，因此每个帧槽固定为 **320 样本**。
- `Frame.Size` 是包含 1 字节帧头的 octet-aligned 存储帧长度；它只用于校验和记账，不用于累加样本。
- 模式从 FT=0 切换为 FT=8 等变化不会产生样本空洞，也不会被计为丢帧。
- `Frame.Index` 是单调的 20 ms 帧槽索引。索引跳号才表示真实丢帧，跳过的槽各补 320 个隐藏样本。
- Q=0 的坏帧只消耗当前一个槽。下一帧的 `SampleStart` 继续加 320，不会整体偏移。
- FT=14（`SPEECH_LOST`）记为真实丢帧；FT=15（`NO_DATA`）记为 DTX/无数据槽；两者都保留时间线位置。

因此最终满足：

```text
Stats.Samples == Stats.FrameSlots * 320
```

## 使用

解析 RFC 4867 单声道 AMR-WB 存储文件：

```go
timeline, err := amrwb.ParseStorage(file)
if err != nil {
    return err
}
for _, span := range timeline.Spans() {
    // span.SampleStart, span.SampleEnd, span.Mode, span.Disposition
}
```

从外部帧元数据建时间线：

```go
timeline := amrwb.NewTimeline()
span, err := timeline.Add(amrwb.Frame{
    Index:   frameIndex, // 连续帧只加 1，与编码模式无关
    Mode:    amrwb.Mode660,
    Size:    18,         // 包含帧头的总长度
    Quality: amrwb.QualityGood,
})
```

若下一帧 `Index` 为 `frameIndex+2`，时间线会在两者之间生成一个
`DispositionLost`、`Concealed=true` 的 320 样本槽。

## 模式帧长

| FT | 模式 | 总字节（含帧头） |
|---:|------|---:|
| 0 | 6.60 kbit/s | 18 |
| 1 | 8.85 kbit/s | 24 |
| 2 | 12.65 kbit/s | 33 |
| 3 | 14.25 kbit/s | 37 |
| 4 | 15.85 kbit/s | 41 |
| 5 | 18.25 kbit/s | 47 |
| 6 | 19.85 kbit/s | 51 |
| 7 | 23.05 kbit/s | 59 |
| 8 | 23.85 kbit/s | 61 |
| 9 | SID | 6 |
| 14 | SPEECH_LOST | 1 |
| 15 | NO_DATA | 1 |

运行验证：

```sh
go test ./...
go vet ./...
```

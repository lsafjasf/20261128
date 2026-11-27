# 20261128

## 24-bit 到 16-bit 瞬态审计

这个 Go 工程比较三种量化路径，并给出修正后的 16-bit WAV：

- 位深截断：直接丢弃 24-bit 样本的低 8 位。
- TPDF 抖动：零均值三角概率分布抖动后量化。
- 二阶噪声整形：TPDF 抖动加二阶误差反馈，把误差能量推离低频。
- 修正路径：量化前预留余量，再使用二阶噪声整形，避免整形过冲撞到全幅。

### 运行

```bash
go test ./...
go run ./cmd/transient-lab -out out
```

默认会生成一个包含低频持续信号和两个鼓点的 24-bit WAV。分析真实文件时传入单声道 24-bit PCM WAV：

```bash
go run ./cmd/transient-lab -in input-24bit-mono.wav -out out -headroom-db -1
```

### 输出

- `out/summary.csv`：峰值、瞬态能量、起振斜率、瞬态窗和非瞬态区残差、重复残差相关、高通残差。
- `out/*-frames.csv`：5 ms 帧级 RMS 与残差包络，并标记鼓点瞬态窗口。
- `out/*-16bit.wav`：各量化路径的可听结果。
- `out/report.md`：Markdown 汇总表和判读说明。

不要只看静态频谱。鼓点冲击主要依赖时间域的峰值、起振斜率、瞬态窗能量和短峰值残差；`repeat_correlation` 为 1 表示截断误差对同一输入完全确定且可重复，而 TPDF 抖动的独立运行残差应近似不相关。

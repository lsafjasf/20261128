# 24 fps → 60 fps judder 分析器

纯 Go 实现（无第三方依赖）。对源帧序列做整数时间基重采样，逐帧计算每个
重复/合成帧的来源与时间戳，并输出 judder 曲线，区分**缺帧停顿**与
**插帧抖动**。

## 时间基

取源帧率与输出帧率的最小公倍数作为 tick 时间基：

- 24 fps、60 fps → 120 tick/s
- 源帧周期 5 tick（41.6667 ms），输出槽周期 2 tick（16.6667 ms）

所有 PTS、步长、相位误差先在整数 tick 域计算，再换算毫秒，因此
“等步长”判定不会引入浮点噪声。

## 重采样策略

- `repeat`：输出槽映射到最近的前序源帧，无新帧时复制（duplicate）。
- `blend`：落在两个源帧之间时做线性插值（synthesized，记录两侧源帧索引
  与 alpha）；跨越源缺口或超出源尾部时退化为保持（gap_hold / tail_hold）。
- 缺口策略 `hold`：缺失源帧区间内保持最后一个可用帧。

## judder 分类

- `repeat_pause`：复制帧，有效 PTS 步长为 0（画面静止）。
- `repeat_jump`：复制策略下恢复真实运动时的追赶大步。
- `missing_source_pause`：源缺帧导致的保持帧停顿。
- `missing_source_jump`：缺口结束后第一个真实帧的追赶跳变。
- `interpolation_judder`：合成帧步长偏离理想值（24→60 线性插帧下为 0）。
- `source_end_hold`：源素材结束后的尾部保持，不计入 judder/RMS。
- `smooth`：步长恰为一个输出周期；`bootstrap` 为首帧占位。

## 用法

```bash
# 完整源序列：repeat（调整前）对比 blend（调整后）
go run ./cmd/judder -out-dir artifacts

# 源第 12、13 帧缺失的场景
go run ./cmd/judder -out-dir artifacts -gaps 12-13 \
  -prefix missing -suffix missing
```

常用参数：`-src-fps`、`-out-fps`、`-source-frames`、`-gaps`（逗号分隔的
区间，如 `12-13,20`）、`-before-mode`、`-after-mode`、`-interpolation`、
`-missing-policy`。

## 产物

- `repeat-frames.csv` / `blend-frames.csv`：每个输出帧一行，含 PTS、
  kind、源帧索引、alpha、有效 PTS、相位误差、实际/理想步长与分类。
- `judder-comparison.json`：两种策略的完整报告（帧表、曲线、汇总）与
  delta；缺帧场景对应 `missing-*` 命名。
- `judder-curves.svg`：归一化步长误差曲线（负为停顿、正为跳变）。

## 验证

```bash
go test ./...
```

# srtplan

两条（或更多）SRT 在同一时间区间重叠时，旧做法只是把文字简单拼接，导致双语字幕串成一行、
说话人标签混进正文。`srtplan` 改为按 **时序 → 区域 → 语言 → 人工优先级** 生成可解释的显示方案：

- 不同语言永远不合并到同一行，按区域上下分屏；
- 说话人标签解析为结构化字段，渲染时单独保留；
- 区域被占满时把 cue **拆分**（同 cue 的多个片段编号）并把抢不到区域的片段 **延后** 到最近空闲槽位；
- 任何带文字的 cue 都不会被删除，引擎内置覆盖率校验，丢内容直接报错。

全部逻辑均在 Go 工程内，仅使用标准库。

## 工程结构

| 路径 | 职责 |
| --- | --- |
| `internal/srt` | SRT 解析与渲染，兼容 BOM、CRLF、时间戳坐标扩展 |
| `internal/language` | 按书写脚本检测中/英/日/韩/俄/阿拉伯语 |
| `internal/plan` | 排版引擎：时间窗切分、区域分配、拆分与延后、覆盖率校验、JSON 报告 |
| `cmd/srtplan` | 命令行入口 |
| `testdata` | 端到端示例双轨字幕 |

## 决策规则

引擎先在所有 cue 的起止时刻切分时间窗，每个时间窗内按以下顺序排序并依次占用
`top` / `bottom` 两个区域：

1. **人工优先级**：带 `[manual]` 或 `[manual:N]` 的 cue 永远高于自动 cue，`N` 大者胜；
2. **显式区域**：带 `[top]` / `[bottom]` 标记或时间戳坐标的 cue 优先满足其偏好；
3. **语言主次**：主语言轨（`-primary`）默认占底部，翻译/次轨默认占顶部；
4. **来源顺序与起始时间**：仍相同则按输入顺序、原始起始时间、cue ID 兜底，保证结果确定。

抢不到任何区域的片段进入延后队列，在其原始窗口结束之后寻找两个区域中最早的空闲槽位，
保留完整文案，仅调整时间，并在报告中同时记录原始时间与实际时间。

Cue 内支持的标记（解析后从正文移除）：

- `[manual]`、`[manual:9]`：人工校对与人工优先级；
- `[top]`、`[bottom]`：区域偏好；
- 说话人标签：`ALICE:`、`NEWS ANCHOR:`、`Alice (V.O.):`、`[Narrator]:`。
  普通句首大写（如 `Hello: welcome ...`）不会被误判为说话人。

## 用法

```bash
go build ./cmd/srtplan

srtplan \
  -primary testdata/primary.srt \
  -secondary testdata/secondary.srt \
  -secondary-name english \
  -report plan.json \
  -out merged.srt \
  -max-deferral-ms 4000
```

输出：

- `merged.srt`：可直接播放的字幕，顶部/底部 cue 带 `Y1/Y2` 坐标，不会叠在同一基线；
- `plan.json`：每个片段的区域、语言、说话人、是否拆分/延后、决策码与人类可读原因，
  以及逐 cue 的覆盖率统计（`placedMs`、`deferredMs`、`complete`）。

决策码取值：`placed`、`split_placed`、`deferred`、`split_deferred`、
`manual_placed`、`manual_deferred`。

## 不丢内容保证

`plan.Stats.DroppedCues` 恒为 0；`verifyNoContentDropped` 会核对每个有内容 cue 的
`placedMs + deferredMs >= 原始时长`，不满足即返回错误而不是静默丢弃。空 cue（无文字）
不参与排版，也不计入内容 cue。

## 测试

```bash
go test ./...
```

覆盖 SRT 解析/渲染、语言识别、双语分区、三轨冲突拆分+延后、人工优先级、说话人识别、
空 cue 处理与 CLI 端到端冒烟。

# subburn — 字幕烧录时序定位与修复

预览里字幕准时，成片却每过几十秒进一步提前，通常不是字幕文件本身在变化，
而是**烧录时用于选择字幕的时钟**和**最终画面真实显示的时钟**不是同一条。
误差会按时间缩放，因此整轨加一个固定偏移只能让某一时刻对齐，其他地方仍错。

本项目是纯 Go 标准库实现，把问题拆成四层并逐层验证：

1. `Rate`：用精确分数帧率（如 `30000/1001`）计算帧 PTS，不累加整数毫秒。
2. `RenderClock`：分别保存 `TrueTime(frame)` 和 `QueryTime(frame)`。
3. `Pipeline`：比较预览与成片里每个字幕 cue 的真实显示起点。
4. `BurnPlan`：字幕保持源时间，预滚只进入编码 PTS，查询前精确减去预滚。

## 误差归因

`DiagnosePipeline` / `subburn audit` 对整组字幕起点拟合三类模型，而不是只看首尾：

| 观测误差形态 | 归因层 | 含义 |
| --- | --- | --- |
| 随源时间线性增长 | `frame_timebase` | 帧率/时间基错误，或逐帧累加了固定毫秒步长 |
| 在分段边界呈阶梯跳变 | `encoder_preroll` | 每个编码段都重复叠加了预滚 |
| 所有 cue 近似同一常量 | `subtitle_origin` / `output_display_time` | 是起点未归一化或容器显示起点错误，不是累计误差 |
| 误差被限制在一帧左右 | `none` | 正常帧量化，没有系统性漂移 |

关键区分：

- 编码预滚**只在字幕查询钟里出现一次**时，是常量平移；只有按分段反复计入，
  才会形成累计阶梯。
- 预览和成片帧率不同本身不一定漂移。只要输出帧仍以精确 PTS 映射源时间，
  差异只会落在半帧到一帧的量化内。
- `29.97` 不是 `30000/1001`。本工具保留用户输入的小数精度，不静默暗换。

## 修正原则

`Pipeline.Corrected()` 只清除时钟缺陷，不改字幕内容：

- `FrameStep = 0`，帧时间从精确有理数率逐帧计算。
- `ReportedRate` 清零，不再用名义帧率伪造 PTS。
- 字幕查询使用归一化后的 `DisplayPTS`。
- `Preroll` 仍保留在编码侧：`EncodedPTS = DisplayPTS + Preroll`，
  而 `SubtitleQueryTime(EncodedPTS) = EncodedPTS - Preroll`。
- `SubtitleOrigin`、`DisplayStart` 只允许在导入/封装入口归一化一次；
  不允许在最终文件上统一加减补偿。

`BuildBurnPlan` 对每个 cue 独立映射 `[Start, End)` 到精确帧号。没有全局
offset 参数，也没有“漂移修正率”这种会再次引入时间依赖的参数。

## 运行

```bash
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/subburn demo
```

演示输出会同时给出预览/成片起点和归因。当前复现场景使用
`30000/1001` 素材、每帧 `34ms` 的错误时间戳步长：

```text
rate: preview=30000/1001 export=30000/1001 exact-frame=33.366666ms
cue       expected      final        error
cue-01    10.01s        9.843s        -167ms
cue-07    2m10.03s      2m7.594s      -2.436s
cue-13    4m10.016s     4m5.345s      -4.671s
root: frame_timebase cumulative=true
slope: -1.8700%, drift=-1.122s per minute, linear-rmse=9.7928ms
fix: exact PTS + preroll excluded from lookup; cue starts unchanged; max residual=0s
```

第一句只早 167ms，最后一句早 4.671s，说明误差按时间累计。若在最终文件加
固定偏移，无论取哪个值都不可能同时修好这两句。

## 从 SRT 生成帧级计划

```bash
go run ./cmd/subburn plan \
  -rate 30000/1001 \
  -duration 5m \
  -preroll 2s \
  -srt subtitles.srt \
  -json
```

输出每个 cue 的：

- `first_frame` / `end_frame`（`end_frame` 为排他边界）
- `display_start_ns` / `display_end_ns`
- `encoded_start_ns` / `encoded_end_ns`

编码器或滤镜必须用 `encoded_pts - preroll` 查字幕，不能直接拿编码 PTS 查。
这是源层的预滚修正，不会改动 SRT 起点。

## 用实际成片采样做归因

`audit` 接受预览/成片字幕起点的实测样本（毫秒）。样本应覆盖整段节目，
至少包含两个不同时间段；覆盖越长，线性漂移与分段阶梯越容易区分。

```json
{
  "preview_rate": "30000/1001",
  "export_rate": "30000/1001",
  "frame_step_ms": 34,
  "reported_rate": "",
  "preroll_ms": 2000,
  "preroll_in_subtitle": false,
  "segment_ms": 0,
  "segment_preroll_ms": 0,
  "subtitle_origin_ms": 0,
  "display_start_ms": 0,
  "display_in_subtitle": false,
  "samples": [
    {"id": "cue-01", "expected_ms": 10010, "observed_ms": 9843},
    {"id": "cue-07", "expected_ms": 130030, "observed_ms": 127594},
    {"id": "cue-13", "expected_ms": 250016, "observed_ms": 245345}
  ]
}
```

```bash
go run ./cmd/subburn audit -json samples.json
go run ./cmd/subburn audit samples.json   # 人类可读报告
```

报告中的 `slope` 是“秒误差/秒源时间”，`drift` 是每分钟累计量，
`linear-rmse` 与 `step-rmse` 用来判断数据更符合线性还是分段阶梯。
若 `segment_ms` 未知，工具会从平坦段与跳变的分布推断编码段周期；
只有推断出的阶梯模型残差也落在一帧容差内，才会归因为 `encoder_preroll`。
`cumulative=false` 的 `subtitle_origin` / `output_display_time` 不得当作
累计误差去套整轨偏移。

## 代码结构

- `rate.go`：精确分数帧率、帧 PTS、半开区间帧边界。
- `clock.go`：真实显示钟、字幕查询钟、预滚与分段预滚模型。
- `pipeline.go`：预览/成片逐 cue 起点比较。
- `diagnose.go`：常数、线性、分段阶梯拟合与层归因。
- `plan.go`：无全局偏移的帧级烧录计划。
- `srt.go`：SRT 读取/写回，时间按源显示钟解释。
- `cmd/subburn`：演示、实测审计、SRT 到帧计划。

## 回归覆盖

- `30000/1001` 在 30000 帧处精确等于 1001 秒。
- 固定毫秒步长会累计，精确 PTS 不会。
- 29.97 与 30000/1001 不被混同。
- 线性提前归因到帧时间基。
- 分段重复预滚归因到 `encoder_preroll`，且是阶梯而非直线。
- 未提供分段周期时，仍能从重复跳变推断周期并验证阶梯模型。
- 单次预滚、字幕原点、输出显示起点只归因为常量层。
- 修正后所有 cue 与预览误差不超过一帧，SRT 起点未被整体平移。

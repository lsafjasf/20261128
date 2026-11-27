# interlace-diag

逐帧诊断隔行素材与去隔行结果中的三类常见问题：

- `source_motion_blur`：原始画面本身就有方向性运动模糊。
- `weave_error`：场被错误组合或隔行画面被当成逐行显示，留下梳状/拉链状残差。
- `over_smoothed`：去隔行做了行复制、混合或过度插值，画面糊但没有梳齿。

工具只使用 Go 标准库，不引入任何第三方 Go 依赖。它不内置 H.264/HEVC 解码器；请先把素材解成 Y4M 或 headerless planar YUV。

## 判别方法

每帧都会拆成偶行场（top）和奇行场（bottom），并同时检查：

| 证据 | 检测方法 | 主要用途 |
| --- | --- | --- |
| 梳状残差 | 在水平梯度边缘上计算 `abs(2*p(y)-p(y-1)-p(y+1))`，统计交叉场异常比例和归一化幅度 | 识别错误 weave / 仍有交错 |
| 场内/帧间位移 | 对两场的水平、垂直边缘投影做有符号互相关，输出 `px/场`（或帧间 `px/帧`）、方向和置信度 | 判断运动方向与速度；逐行素材自动改用帧间位移 |
| 场序 | 比较上一帧 top/bottom 与当前帧 top/bottom 的连续位移一致性，选择 TFF 或 BFF | 防止把时间顺序判断错 |
| 方向清晰度 | 在 top/bottom 场内分别测量垂直边和水平边的等效过渡宽度 | 区分锐利 weave、方向性运动模糊和整体平滑 |
| 行重复 | 相邻行差异相对下一行差异过小；同时统计 top/bottom 场重复率 | 识别去隔行行复制/场复制 |
| 垂直细节比 | 场内垂直高频 / 水平高频 | 识别垂直方向的细节塌陷 |

判定不是只看“糊不糊”，而是看证据组合：

- weave 错误：交叉场梳状残差高，且方向性模糊不足以解释这些残差。
- 原始运动模糊：边缘在运动方向上明显变宽，宽边方向与有符号场内/帧间位移一致，且没有明显的行重复。
- 过度平滑：行重复、场重复或方向细节下降明显，原始交叉场差异可由行复制解释。

运动模糊素材在隔行采集中仍可能产生一定的奇偶行差异，因此报告保留原始 `combing.score`，同时用 `hypothesis_scores` 给出经过互斥证据修正后的三假设分数。

当前主判定门槛：`combing.score >= 0.48` 且交叉场边缘占比 `>= 10%` 优先判 weave；运动模糊需方向对齐分数 `>= 0.50` 且行重复率 `< 25%`；过度平滑分数 `>= 0.42`。其余情况根据锐利度给 `clean` 或 `inconclusive`。

## 输入

### Y4M

支持 8-bit `C420*`、`C422`、`C444`、`Cmono`。只分析 Y 平面，色度数据会被正确读取和跳过。Y4M 头里的 `It`/`Ib` 会作为场序提示，但最终优先使用相邻场运动测得的场序。

```bash
go build -o interlace-diag .
./interlace-diag -i interlaced.y4m -output-format jsonl > evidence.jsonl
```

### Headerless YUV

```bash
./interlace-diag \
  -i input.yuv -format raw -size 1920x1080 \
  -pix-fmt yuv420p -fps 29.97 \
  -output-format text
```

`-pix-fmt` 支持 `yuv420p`、`yuv422p`、`yuv444p`、`gray`。也可以从 stdin 读取：

```bash
cat input.y4m | ./interlace-diag -i - -format y4m -output-format jsonl
```

## 输出

默认 `jsonl`：每个帧一行，最后一行是 summary。

```json
{"type":"frame","frame":12,"time_seconds":0.48,"verdict":"weave_error","confidence":0.97,"field_order":{"order":"tff","confidence":0.91,"source":"measured"},"hypothesis_scores":{"weave_error":0.94,"source_motion_blur":0.08,"over_smoothed":0.05},"evidence":{"combing":{"edge_fraction":0.31,"magnitude":0.47,"same_parity_residual":0.04,"score":0.94},"blur":{"edge_width_horizontal":2.1,"edge_width_vertical":2.0,"detail_horizontal":9.4,"detail_vertical":9.1,"direction":"sharp","directional_ratio":1.05},"smoothing":{"line_duplication_rate":0,"vertical_detail_ratio":1.02,"temporal_field_duplication":0.1,"score":0.03},"fields":{"top_bottom_mad":16.2,"motion_compensated_mad":3.8,"residual_ratio":0.23,"temporal_mad":18.7},"motion":{"intra_field":{"x_px":2,"y_px":0,"speed_px":2,"direction":"right","confidence":0.88},"inter_frame":{"x_px":4,"y_px":0,"speed_px":4,"direction":"right","confidence":0.90},"speed_px_per_field":2,"direction":"right","confidence":0.88}},"reasons":["结论：运动边缘存在跨场奇偶行梳状残差，优先判为 weave 错误。","场序 TFF；注意场序只解释时间顺序，不直接等同于 weave 质量。"]}
{"type":"summary","frames":240,"counts":{"weave_error":198,"source_motion_blur":34,"over_smoothed":8},"dominant_verdict":"weave_error","field_order":"tff","field_order_confidence":0.93}
```

字段含义：

- `verdict`：`weave_error`、`source_motion_blur`、`over_smoothed`、`clean` 或 `inconclusive`。
- `confidence`：当前结论的置信度，不是图像质量的绝对分数。
- `field_order.source`：`measured`、`declared`、`sequence_prior`、`insufficient_motion`。
- `combing.edge_fraction`：参与统计的边缘像素中，交叉场残差显著的比例。
- `combing.magnitude`：归一化交叉场残差幅度。
- `combing.same_parity_residual`：同场奇偶行残差；与交叉场残差对照可减少普通纹理误报。
- `blur.edge_width_*`：等效边缘过渡宽度，单位像素；越大越糊。
- `blur.detail_*`：场内平均梯度，用于辅助判断方向性。
- `smoothing.line_duplication_rate`：相邻行近乎复制的比例。
- `smoothing.temporal_field_duplication`：两场在边缘位置几乎相同的比例。
- `fields.*_mad`：原始/运动补偿后的场差，`residual_ratio` 越低说明两场对齐越好。
- `motion.intra_field`：相邻 top/bottom 场的原始位移。
- `motion.inter_frame`：相邻整帧位移，用于已经去隔行的素材。
- `motion.speed_px_per_field`：最终采用的等效场速度；优先场内位移，场内无运动时用 `inter_frame / 2`。换算像素/秒约为 `speed * frame_rate / 2`。

其他输出模式：

```bash
./interlace-diag -i in.y4m -output-format json   # 单个 JSON 文档
./interlace-diag -i in.y4m -output-format text   # 每帧一行可读摘要
```

## 参数

```text
-input, -i       输入文件，- 表示 stdin
-format          输入格式：auto、y4m、raw
-size            raw 输入尺寸，例如 1920x1080
-pix-fmt         raw 像素布局
-fps             raw 输入帧率
-field-order     场序提示：auto、tff、bff
-max-shift-x     最大水平场位移搜索范围，0 为自动
-max-shift-y     最大垂直场位移搜索范围，0 为自动
-output-format   报告格式：jsonl、json、text
-output          报告文件，- 表示 stdout
```

## 限制与读法

- 场序需要画面中有可测运动。静止画面会返回 `insufficient_motion` 或较低置信度，这是正常结果。
- 该工具分析解码后的 Y 平面，不判断编码器、播放器或显示端的缩放/锐化。
- 运动模糊是源内容属性，不等于缺陷；报告把它单独列出，避免误判成去隔行失败。
- 低分辨率、大面积纯色、强周期纹理和超慢运动可能降低置信度；应结合连续多帧的 summary 与原始帧复核。

## 验证

```bash
go test ./...
go vet ./...
```

测试覆盖 TFF/BFF、错误 weave、原始方向性运动模糊、行复制过度平滑、清晰逐行画面，以及 Y4M 的 JSONL/JSON 端到端输出。

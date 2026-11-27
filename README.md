# frameburn

审片代理用的逐帧烧录与核验工具。帧号、时间码和渲染共用同一个输出帧索引，
画面内容重复、冻结或剪辑都不会让标签跳号或漂移。实现只依赖 Go 标准库。

## 快速开始

```bash
go test ./...
go run ./cmd/frameburn demo -out out -width 320 -height 180 -frames 90
```

`demo` 会生成 `23.976` 与 `29.97DF` 两套逐帧 P6 PPM，并立即做独立核验。
每套目录包含：

- `frame_000000.ppm` 等逐帧画面，标签固定在左上角。
- `frames.jsonl` 每帧的索引、时间码、内容帧、剪辑段和事件状态。
- `manifest.json` 分辨率、速率、事件清单和内容区位置。
- `report.json` 标签、黑帧、剪辑断点、暂停区间的核验结果。

## 命令

```bash
# 默认事件：暂停、2 帧黑场、剪辑点、再次暂停
go run ./cmd/frameburn generate -out out/29.97df \
  -rate 29.97DF -width 480 -height 270 -frames 150

# 自定义零基输出帧事件，可重复
go run ./cmd/frameburn generate -out out/custom -rate 23.976 \
  -width 320 -height 180 -frames 120 \
  -pause 30:24 -black 60:2 -cut 72 -pause 90:8

# 独立复核已有目录
go run ./cmd/frameburn verify -dir out/29.97df
```

时间线 JSON 也可直接传给 `-config`：

```json
{
  "rate": "29.97DF",
  "width": 320,
  "height": 180,
  "frames": 120,
  "events": [
    {"kind": "pause", "start": 30, "count": 24},
    {"kind": "black", "start": 60, "count": 2},
    {"kind": "cut", "start": 72},
    {"kind": "pause", "start": 90, "count": 8}
  ]
}
```

事件和帧号均为零基。`black`/`pause` 使用 `START:COUNT`，`cut` 位于目标帧开头。
暂停从目标帧开始重复上一内容帧，因此暂停区间的画面完全相同，但烧录标签继续步进。

## 步进规则

- 所有换算以整数输出帧索引为唯一输入，不使用逐帧浮点累加。
- `23.976`：真实速率 `24000/1001`，标签按 24 fps 非丢帧编号。
- `29.97DF`：真实速率 `30000/1001`，标签按 30 fps 编号，每分钟开头丢
  `;00`、`;01`，但每第 10 分钟不丢。边界如帧 `1800` 为 `00:01:00;02`，
  帧 `17982` 为 `00:10:00;00`。
- `29.97ND` 也支持，用于对照非丢帧输出。
- 标签格式为 `F000001 TC 00:00:00;01 29.97DF`；DF 标签用分号，ND 用冒号。

## 核验方式

核验器读取 `manifest.json` 和 `frames.jsonl`，重新构建期望计划并逐帧执行：

- 重新计算帧号/时间码，并将烧录面板与期望像素逐点比较，防止标签漂移。
- 只在标签以外的内容区计算黑帧，避免黄色边框和文字干扰。
- 用相邻内容区平均 RGB 差检测剪辑点；默认阈值为 `28`。
- 用相邻内容区近似相等检测暂停/冻结；默认阈值为 `0.25`。
- 将检测到的黑帧区间、剪辑帧和暂停区间与清单事件精确比对。

默认阈值位于 `internal/audit.DefaultOptions`。调整阈值时可调用
`audit.Audit(dir, audit.Options{...})`。

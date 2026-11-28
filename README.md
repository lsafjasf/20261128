# 20261128

Repository initialized for 20261128.

## fieldcut

隔行素材切点联合模型（Go）：场序 × 切点 × 输出帧率。

- 切点以源场为单位（`Cut{In, Out}`，左闭右开）。
- 联合模型判定（`NormalizeCut`）：
  - 输出每帧消耗 2 场（隔行输出 / 帧率逐行输出）→ 非场对边界的切点必须外扩到 field pair，否则接缝帧会把两个源帧的场织在一起，产生一场闪烁；
  - 输出每场一帧（field-rate 逐行）或时间戳重采样 → 奇数场修剪合法，原样保留。
- `NormalizeTimeline` 处理相邻镜头外扩后的重叠，保证同一场不被输出两次。
- `VerifyFields` 逐场核对：PTS 步进严格等于一场时长、场对完整性（flicker 检测）、dominance 交替。

```sh
go test ./...
go run ./cmd/fieldcut -mode interlaced -v        # 复现闪烁并展示修复
go run ./cmd/fieldcut -mode progressive-field    # 合法奇数场修剪
```

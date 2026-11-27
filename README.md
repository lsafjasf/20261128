# Timeline marker identity

本仓库用 Go 实现一套不会因名称、帧位或颜色相同而丢失 marker 的身份模型。核心规则是：

> `Marker.ID` 是身份；`Owner` 决定解析域；`Name`、`Range`、`Color` 只是可变属性。

因此同一帧上两个都叫 `Color`、三个都叫 `Audio` 的 marker，只要 ID 不同，导出和重导入后仍然是五个 marker。

## 三类身份

| Scope | 稳定 owner | range 坐标系 | 生命周期 |
| --- | --- | --- | --- |
| `media` | `mediaId` | 媒体源帧 | 跟随媒体资产；可投影到使用该媒体的多个 clip |
| `clip` | `sequenceId + clipId` | clip 局部帧（从源入点开始） | 只属于该序列中的该 clip 实例 |
| `global` | `sequenceId` | 序列时间线帧 | 只属于该序列 |

`Owner.Validate` 强制每类身份只能带本域字段。例如 clip marker 必须同时有 `sequenceId` 和 `clipId`，media marker 不能偷偷携带序列 ID。

Marker ID 是 opaque、稳定的字符串，不由名称、帧位、颜色或 owner 计算。显式 ID 在导出、颜色更新、合并和重导入时保持不变；新建时未给 ID 才生成随机 ID。

## 同一帧、同一名称

导出格式是规范化的 `Document`：

```json
{
  "schemaVersion": 1,
  "media": [],
  "sequences": [],
  "clips": [],
  "markers": [
    {
      "id": "m-global-color-a",
      "name": "Color",
      "owner": {"scope": "global", "sequenceId": "seq-main"},
      "range": {"start": 42, "end": 43},
      "color": "#E53935",
      "revision": 1
    }
  ],
  "aliases": []
}
```

数据库或导出器不应再以 `(name, frame)` 建唯一键。正确唯一键是 `marker.id`。若数据库仍需要秒级索引，应建立非唯一索引 `(owner, start, end)`。

## 重叠与合并

- `Project.At(sequenceID, frame)` 返回该帧所有可见 marker，包括通过每个匹配 clip 投影出的 media marker。
- `Project.Overlapping(owner, range)` 返回相交 marker，重叠不会自动删除或合并。
- `Project.Merge(ids, options)` 才执行显式合并。

合并规则：

1. 所有源 marker 必须属于同一 owner；跨 owner 合并拒绝。
2. range 取并集；`RequireOverlap` 开启时，输入的 range 必须连通。
3. 名称相同才自动沿用，否则必须提供 `MergeOptions.Name`。
4. 颜色相同才自动沿用，否则必须提供 `MergeOptions.Color`；冲突时返回 `ErrNameConflict` 或 `ErrColorConflict`。
5. 保留字典序最小的源 ID，其余源 ID 写入 `aliases`，并永久指向规范 marker。

示例：

```bash
go run ./cmd/markerctl demo > markers.json
go run ./cmd/markerctl validate markers.json
go run ./cmd/markerctl resolve markers.json m-media-audio-a seq-main
```

`demo` 会导出同帧的 2 个 `Color` 和 3 个 `Audio`，分别覆盖 global、clip、media 三种 owner，并立即做一次内存重导入校验。

## 颜色变化

`Project.SetColor(id, color)` 只更新可变属性：

- 不改变 ID、owner、range，也不创建新 marker。
- 传入合并前的旧 ID 时，先解析 alias，再更新规范 marker。
- `revision` 单调增加，`colorHistory` 保存每次颜色变化。
- 导入时拒绝回退的 revision、非法颜色，以及“当前颜色不等于最后一条历史颜色”的损坏数据。

合并产生颜色冲突时必须显式选择颜色，避免按导入顺序随机取一个。合并后的历史按 revision 和来源 ID 排序并规范化，随后重导入仍可校验。

## 重导入与引用

`marker.Load(doc)` 先完整校验，再原子替换内存图；失败不会留下半导入状态。导入时：

- 所有 live marker ID 必须唯一。相同名称、相同帧位不是错误。
- 所有 media、sequence、clip owner 必须存在，range 必须落在 owner 时长内。
- alias 链必须最终指向 live marker，不能有环，`from` 不能和 live ID 冲突。
- `Project.Get(id)`、`ResolveID(id)`、`Resolve(Reference{...})` 都会沿 alias 链解析，因此合并前保存的引用在重导入后仍有效。

## 无解的跨序列引用

以下情况不会猜测或自动修复，而是返回 `ErrCrossSequence` 或导出带字段的结构化诊断：

- clip marker 的 owner 是 `(seq-a, clip-a)`，却从 `seq-b` 解析。clip instance 只在原序列存在，无法跨序列引用。
- global marker 属于 `seq-a`，却从 `seq-b` 解析。
- media marker 在目标序列中没有使用该媒体的 clip，或其 range 不在任何 clip 的 `[sourceIn, sourceOut)` 内。
- 导入时 marker 声称 clip 属于 `seq-a`，但同名 clip ID 实际存在于 `seq-b`。诊断码为 `cross_sequence_reference`。

`ValidationError.Diagnostics` 会同时保留多个问题，并带 `markerId`、`ownerScope`、`mediaId`、`sequenceId`、`clipId`，可直接显示在导入对话框。其他诊断码包括 `missing_media`、`missing_clip`、`missing_sequence`、`alias_cycle`、`alias_target_missing`、`marker_outside_owner`。

跨序列复用应使用 media marker，或先显式复制/重建 clip marker。把 clip marker 静默迁到另一序列会改变其语义，本实现刻意拒绝。

## 时间语义与限制

- 所有帧都是整数半开区间 `[start, end)`，同一帧不会因区间相接而误判重叠。
- clip marker 使用 clip 局部帧；移动 placement 不影响局部 range。
- media marker 到序列的投影当前是 1:1 源帧映射，不处理变速、倒放或时间重映射。需要这些效果时，应在投影层扩展，而不是改变 marker 身份。
- 若后续 trim 使已有 marker 越出 owner 边界，导入会报 `marker_outside_owner`，不会静默裁掉 marker。
- 合并保存来源 ID 的 alias、range 并集和规范化颜色历史，但不复制其他未来扩展属性；新增属性若要跨合并保留，需扩展合并器。

## 验证

```bash
go test ./...
go vet ./...
```

测试覆盖：同帧同名五个 marker 的 JSON 往返、重叠不自动合并、media marker 多 clip 投影、显式合并且旧引用可解析、颜色变化历史、跨序列诊断、alias 环检测和独立导入错误聚合。

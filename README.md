# mxfindex

纯 Go、仅标准库实现的 MXF 随机访问审计与索引重建工具。它顺序扫描 KLV，
解析 partition pack、Random Index Pack、Index Table Segment、essence element
和 SMPTE 12M 时间码组件，生成可寻址的 edit-unit 区间以及可直接消费的重建索引。

核心约束：**只有完整重建才返回 `status: "complete"` 和 `success: true`。**
缺段、索引表项不足、覆盖空洞、重复 edit unit、无法映射到 essence 的
`StreamOffset`、partition 链错误等都会使结果变为 `incomplete`。

## 解决的问题

MXF 播放器通常用 `IndexSID` 找到索引表，再把 `StreamOffset` 通过 `BodySID`
映射到实际 partition 中的 essence 字节。若索引覆盖或 partition 边界与文件
实际布局不一致，部分 edit unit 能定位，另一部分会 seek 失败并退回起点。

本工具会同时检查两侧：

- 索引侧：`IndexStartPosition`、`IndexDuration`、`IndexEntryArray`、
  `EditUnitByteCount`、`IndexSID`、`BodySID`。
- 文件侧：`ThisPartition`、`PreviousPartition`、`FooterPartition`、
  `BodyOffset`、实际 KLV 边界、RIP 指向的 partition。
- 内容侧：essence element 的实际文件偏移、body container 边界和开始时间码。

## 命令行

```bash
# 人读摘要；不完整时退出码为 2
go run ./cmd/mxfindex clip.mxf

# 输出完整 JSON 报告
go run ./cmd/mxfindex -json clip.mxf > report.json

# 重建索引并把报告原子写入文件
go run ./cmd/mxfindex rebuild -out report.json clip.mxf
```

退出码：

- `0`：重建索引完整，覆盖所有已识别的 edit unit。
- `2`：报告已生成，但索引不完整，`success` 必须为 `false`。
- `1`：文件无法读取或连 header partition 都找不到。

`-out` 先写同目录临时文件再 rename，不会留下半截 JSON。该命令不会原地改写
MXF；重建结果作为 sidecar JSON 交付，避免在未确认私有 partition 扩展语义时
破坏原始文件。

## JSON 报告

主要字段：

- `partitions`：每个 partition pack 的绝对偏移、类型、SID 和边界。
- `index_segments`：原始索引段及解析出的 entry 数组；损坏段仍会列出并带错误。
- `essence_elements`：每个 essence/system item KLV 的文件范围与 track 号。
- `timecodes`：时间码组件、start frame、rate、drop-frame 与格式化结果。
- `rebuilt_index.tables`：按 `IndexSID` 重建的 edit-unit 表。每个 entry 是
  一个可直接 seek 的绝对偏移。
- `addressable_ranges`：连续 edit-unit 区间，供播放器判断哪些时刻可安全跳转。
- `issues`：带稳定 `code` 的错误和警告。任一 `error` 都会导致成功状态为 false。

`rebuilt_index.complete`、`success`、每个 table/range 的 `complete` 是独立但
一致的完整性信号。读取方不应只看 JSON 是否生成成功。

### 恢复扫描区间

当文件中完全没有可用索引时，报告仍会按 essence 顺序生成
`source: "essence_scan"` 的发现区间。它们只是字节扫描结果：

- `complete` 永久为 `false`；
- 不能据此声称随机访问索引完整；
- 不能覆盖 `status: "incomplete"`。

有部分索引时，只输出已成功映射的 `source: "index"` 区间，不会用猜测补齐
缺失 edit unit。

## 偏移映射

对索引 entry 的 `StreamOffset`，按同一 `BodySID` 的 partition 求：

```text
candidate = partition.ContainerStart + (StreamOffset - partition.BodyOffset)
```

然后要求 candidate 落在该 partition 的 body container 内，并落在实际扫描到的
KLV element 范围内。KLV fill 造成的少量间隙只允许在 `KAGSize` 容差内落到下
一个 element；否则该 edit unit 标记为不可映射。

固定码率索引使用 `EditUnitByteCount`，变长索引使用每个 entry 的
`StreamOffset`。Avid 的 `2 * duration + 1` entry 布局会按 step 2 读取。
索引段之间必须无缝衔接；段起始不是 0、段间空洞或重叠均判为不完整。

## 验证

```bash
go test ./...
go vet ./...
```

测试夹具在测试进程内构造 KLV，不依赖 FFmpeg 或外部二进制，覆盖：

- 带 run-in、RIP、footer partition 和 Index Table Segment 的完整 OP1a 布局；
- 索引表项少于 `IndexDuration`；
- 完全没有索引，但 essence 可扫描；
- `ThisPartition` 与真实偏移不一致；
- 25 fps 与 29.97 drop-frame 时间码换算。

实现同时用本机生成的 OP1a（单段/多段索引）和 OPAtom 文件做过解析验证；
真实样本不提交到仓库。

## 范围与限制

- 开发与运行均为 Go 标准库，无 CGO、无第三方运行时依赖。
- 支持 SMPTE 377M/379M 常见 partition/index KLV 与 Generic Container essence。
- 不解析加密 MXF、私有 partition 扩展或非标准索引编码；这些情况会保留
  警告并保守地返回 `incomplete`，而不是猜测成功。
- 本工具重建可寻址模型，不直接重封装 essence，也不重新编码媒体。

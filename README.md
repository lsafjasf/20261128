# Audio Channel Map Validator

用 Go 实现的交付前声道映射校验器。它同时读取**容器中的编码声道顺序**和**外部交付通道图**，生成逐声道映射，并从磁盘回读映射文件生成校验证据。

重点防止 5.1/7.1 成品中 `C` 与 `Ls/Rs` 因顺序不一致而在监听、播出端互换。

## 校验规则

- 容器声道与交付槽位必须一一对应且数量一致。
- 同一源声道被绑定两次，或两个交付槽位指向同一目标，返回 `overlap` 并拒绝。
- 缺少源声道、缺少交付槽位、只绑定部分源声道，返回 `incomplete` 并拒绝。
- 显式绑定把 `C` 指到 `L` 等错误目标时，返回 `mismatch` 并拒绝。
- 只有完整双射才会写出 `mapping.json`、`mapping.csv` 和 `readback.json`。
- 回读会重新解析已落盘的 `mapping.json`，重建编码顺序和交付顺序，并绑定 SHA-256、核对逐行映射、覆盖率和唯一性。

## 构建与运行

```bash
go build -o bin/audiomap-validator ./cmd/audiomap-validator

./bin/audiomap-validator validate \
  --container examples/container-5.1.json \
  --delivery examples/delivery-5.1-center-surround.json \
  --out evidence
```

示例中的编码顺序为 `L R C LFE Ls Rs`，交付顺序为 `L R Ls Rs C LFE`，运行后会明确给出置换关系：

```text
VALID: 6 channels, reordered=true
  delivery 1 L    <- source 1 L
  delivery 2 R    <- source 2 R
  delivery 3 Ls   <- source 5 Ls     [REORDERED]
  delivery 4 Rs   <- source 6 Rs     [REORDERED]
  delivery 5 C    <- source 3 C      [REORDERED]
  delivery 6 LFE  <- source 4 LFE    [REORDERED]
```

也可加 `--json` 输出机器可读结果。命令可省略 `validate` 子命令。

## 输入格式

### 容器编码

原生读取 `WAV`/`RF64` 的 `fmt ` 块，支持 `WAVE_FORMAT_EXTENSIBLE` 的 `dwChannelMask` 和子格式 GUID。声道顺序按 mask 位顺序读取，不会从交付图反推。没有 mask 的 WAV 使用通用标签 `CH1..CHn`。

其他容器可先提取元数据，再传入 JSON 描述：

```json
{
  "container": "MOV",
  "codec": "pcm_s24le",
  "sample_rate": 48000,
  "channel_count": 6,
  "channel_order": ["FL", "FR", "FC", "LFE", "BL", "BR"]
}
```

或使用 `"channel_mask": "0x3f"` 代替 `channel_order`。两者同时存在时必须一致。

### 交付通道图

最简形式是数组：

```json
["L", "R", "Ls", "Rs", "C", "LFE"]
```

带槽位和显式源绑定的形式：

```json
{
  "name": "broadcast-5.1",
  "channels": [
    {"slot": 1, "label": "L", "source": "FL"},
    {"slot": 2, "label": "R", "source": "FR"},
    {"slot": 3, "label": "C", "source": "FC"},
    {"slot": 4, "label": "LFE", "source": "LFE"},
    {"slot": 5, "label": "Ls", "source": "BL"},
    {"slot": 6, "label": "Rs", "source": "BR"}
  ]
}
```

显式源绑定必须全部存在或全部省略。`source_index` 是 0 基索引；`source_position` 是 1 基位置；`source`/`source_ref` 可写别名或 0 基索引。也支持 `{"L": 1, "R": 2, ...}` 这种标签到槽位的对象。

常用别名会归一化，例如 `FL/LEFT -> L`、`FC/CENTER -> C`、`BL/SL/LEFT_SURROUND ->` 左侧环绕族。5.1 中仅有一个左侧环绕声道时，`Ls` 与 `BL/Lrs` 可按上下文匹配；同时存在 side 和 rear 环绕时必须精确匹配。

## 输出证据

- `mapping.json`：容器编码顺序、交付图、逐声道 `source -> delivery` 置换和是否发生重排。
- `mapping.csv`：供人工逐声道核对或下游 remux 使用。
- `readback.json`：映射文件 SHA-256、回读出的两侧顺序、逐行证据及所有检查结果。
- `rejection.json`：仅在拒绝时生成，包含稳定的 `overlap`/`incomplete`/`mismatch`/`invalid` 问题码。

`source_index` 在 JSON 中为 0 基，`source_position`、`delivery_position` 为 1 基；CSV 使用 1 基位置。

## 退出码

- `0`：映射有效，回读校验通过。
- `1`：映射被拒绝，或证据写入/回读失败。
- `2`：参数、容器或交付图读取失败。

## Go 库

```go
layout, err := channelmap.LoadContainer("master.wav")
delivery, err := channelmap.LoadDelivery("delivery.json")
plan, err := channelmap.BuildPlan(layout, delivery)
evidence, err := channelmap.WriteEvidence("evidence", plan)
```

拒绝错误可用 `channelmap.IsRejectionCode(err, channelmap.RejectOverlap)` 或 `errors.As` 判断：

```go
var rejected *channelmap.RejectionError
if errors.As(err, &rejected) {
    // rejected.Issues contains overlap/incomplete/mismatch details
}
```

## 测试

```bash
go test ./...
go vet ./...
```

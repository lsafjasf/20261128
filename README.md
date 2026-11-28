# LTC 跨日时间线

解析已解调的 LTC/SMPTE 12M 80 位码字，识别 drop-frame、二进制组标志和
ST 309 用户位日期，并为单个或多个录像机建立跨日时间线。无法从 LTC 得到
绝对日期的片段不会被猜测，而是保留为显式歧义。

## 构建与运行

```bash
go test ./...
go build -o bin/ltc-timeline ./cmd/ltc-timeline

bin/ltc-timeline -fps 30 A=camera-a.ltc.txt B=camera-b.ltc.txt > timeline.json
cat camera-c.ltc.txt | bin/ltc-timeline -fps 25 C=-
```

主要参数：

- `-fps 24|25|30|0`：标称帧率；`0` 为推断。缺少高帧号、DF 或 ST 309 BGF
  线索时，24/25/30 可能无法唯一区分，取证用途建议显式指定。
- `-bit-order auto|msb|lsb`：仅用于十六进制和打包二进制输入。
- `-input-format auto|text|binary`：强制输入类型。
- `-compact`：输出紧凑 JSON。
- `-output PATH`：写文件，默认写 stdout。

源标签可写成 `LABEL=PATH`；`-` 表示 stdin。多个源必须已有绝对日期且没有
时间线断点，`crossSource.ordering` 才会是 `determined`。

## 输入格式

文本输入按行或按连续流读取：

- 80 个 `0/1`：完整码字，位 0 在字符串最左，位 64..79 必须为
  `0011111111111101`。
- 64 个 `0/1`：仅信息位（无 sync word），常用于 ATC 数据。
- 20/16 个十六进制字符：分别对应 80/64 位。打包字节的位序可自动判定。
- 连续位流：可从 sync word 重新对齐，允许前后有额外位。

二进制输入支持：

- 10 字节完整帧，或 8 字节仅信息位帧。
- 连续打包位流；可在 sync word 前有任意字节前缀。
- 每字节一个 `0/1` 的未打包位流。

这不包含音频 WAV 的双相标记（biphase-mark）解调；输入应是已经恢复的 LTC
数字位流。

## 日期规则

LTC 时间地址本身只有 24 小时时钟，**不包含日期**。本工具只接受以下日期证据：

1. ST 309 日期标志 `BGF2=1, BGF1=0`。
2. 六位 BCD `MJD`：可得到绝对日期。
3. 六位 BCD `DDMMYY`：日、月可验证，但年份只有两位，默认标记为
   `century-ambiguous`，不擅自补成 20xx。

没有日期用户位、标志未置位或日期 BCD 非法的片段：

- 仍按输入顺序和 24 小时回绕生成 `relativeDay`。
- `dateStatus` 为 `absent` 或 `invalid`。
- segment 的 `ambiguous` 为 `true`，`startDate`/`endDate` 缺失。
- `crossSource.ordering` 为 `ambiguous`，不会把两台机位强行排序。

## 时间线语义

每个源输出 `frames`、`segments`、`events` 和 `ambiguities`。

帧级字段包括：

- `timeAddress`：DF 时使用 `HH:MM:SS;FF`。
- `frameNumber`：24 小时内帧序号；29.97 DF 已按每分钟丢 2 个标签计算。
- `dropFrame`、`colorFrame`、`polarity`。
- `binaryGroupFlags` 与 `userBits`（U1..U8，十六进制）。
- `date`：ST 309 解析结果及歧义原因。
- `relativeDay`：已知日期时为相对锚点日期差；否则为按午夜回绕推断。

事件类型：

- `duplicate`：同一天内时间地址重复。
- `rollback`：非午夜回绕的倒退。
- `forward-jump`：跳帧。
- `midnight-rollover`：干净跨午夜。
- `midnight-irregular`：23:59:59:max 到 00:00:00:00 之外的午夜跳变。
- `drop-frame-change`：DF 标志变化。
- `date-rollover`：ST 309 日期在午夜正确前进一天。
- `date-not-advanced`：跨午夜但日期未前进（例如只循环小时）。
- `date-change-without-midnight`、`date-jump`、`date-rollback`。
- `invalid-frame`：sync、BCD 或 DF 地址不合法。

`segments` 在重复、倒退、跳帧、日期矛盾或日期证据类型变化处切分。
`ambiguous=true` 的段不应被当作确定的真实先后顺序。

## 示例结论

对于“A 跳日、B 只循环小时”：

```json
{
  "sources": [
    {
      "label": "A",
      "segments": [{
        "dateStatus": "determined",
        "startDate": "1995-01-01",
        "endDate": "1995-01-02",
        "ambiguous": false
      }]
    },
    {
      "label": "B",
      "segments": [{
        "dateStatus": "absent",
        "ambiguous": true
      }]
    }
  ],
  "crossSource": {
    "ordering": "ambiguous",
    "reason": "B segment 0 has no absolute date"
  }
}
```

退出码只表示解析/IO 是否成功；数据中的歧义是否阻断后续剪辑，应由调用方检查
`ambiguous` 和 `crossSource.ordering`，而不是依赖退出码。

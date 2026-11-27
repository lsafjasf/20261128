# UMP 流事件重建

这个 Go 模块按 **UMP Message Type（MT）** 的规范包长切分 32-bit word，
而不是按 MIDI 1.0 原消息的字节数读取。因此以下消息可以安全混在同一个流中：

- MT `0x2`：MIDI 1.0 兼容通道消息，1 word。
- MT `0x4`：MIDI 2.0 通道消息，2 words，保留完整 16-bit 力度。
- MT `0x4` 的每音符控制器、RPN/NRPN、属性消息等，同样按 2 words 读取。
- MT `0x3`、`0x5`、`0xB`、`0xF` 等多 word 消息，按 MT 尺寸表读取。

保留类型与未知状态不会丢弃：每个 `Event.Words` 都保存原始 word，
并可用 `Known`/`Kind` 判断是否完成语义解码。`ump.Flatten(events)`
可无损拼回完整流的 word 序列（组结束事件不贡献 word）。

## 使用

```go
events, err := ump.Parse(words)
if err != nil {
	var truncated *ump.TruncatedError
	if errors.As(err, &truncated) {
		// events 中仍包含最后一个 Truncated 事件及其原始 words。
	}
}

for _, event := range events {
	fmt.Println(event.Type, event.Group, event.Kind, event.Note, event.Velocity)
}
```

有传输时间戳时使用 `ParseTimed` 或 `ParseTimedWithOptions`：

```go
events, err := ump.ParseTimedWithOptions([]ump.Word{
	{Value: firstWord, Timestamp: 1000},
	{Value: secondWord, Timestamp: 1000},
}, ump.Options{EmitGroupEnd: true})
```

时间戳单位为调用方定义，解析器不缩放或改写。多 word 事件使用第一个 word
的时间戳。`EmitGroupEnd` 会在 UMP Group 改变时和流结束时生成
`KindGroupEnd` 事件，时间戳取该组最后一个 word 的时间戳；无 Group 的
UMP Stream 消息不会切断组。

字节输入默认按 UMP 的大端 word 顺序解析：

```go
events, err := ump.ParseBytes(data)
events, err = ump.ParseBytesOrder(data, binary.LittleEndian)
```

## 尺寸表

```text
MT:     0  1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
words:  1  1  1  2  2  4  1  1  2  2  2  3  3  4  4  4
```

## 测试

测试全部用 Go 构造 UMP word，覆盖混合流边界、7-bit/16-bit 力度、
每音符控制器、未知与保留消息、SysEx 数据、组结束、时间戳和截断包。

```sh
go test ./...
go test -race ./...
```

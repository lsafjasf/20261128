# CEA-608 解码器

纯 Go 标准库实现的 CEA-608 字节流解码器。重点处理控制码单比特翻转、
双字节边界恢复，以及错误后的继续解码。

## 校验与恢复规则

- 每个字节按 CEA-608 要求检查奇校验；低 7 位才是数据。
- `Feed` 跨调用保持字节配对，奇数字节会留到下一次调用，不会自行移动边界。
- 首字节校验失败时丢弃整个字节对，避免控制码的第二个字节被当成普通字符。
- 次字节校验失败时，如果首字节本身是合法可打印字符则保留该字符，否则丢弃整对。
- 坏字节不会进入字幕内存，只产生 `EventError`；下一条合法字幕照常解码。
- 相同且相邻的控制码只执行一次，兼容 CEA-608 控制码重复发送。
- `Reset` 清空待配对字节、字幕内存、重复控制码状态和统计信息。

## 使用

```go
d := cea608.NewDecoder()
for _, chunk := range ccChunks {
    for _, event := range d.Feed(chunk) {
        switch event.Type {
        case cea608.EventText:
            // 新解出的一个或两个字符
        case cea608.EventCaption:
            // EOC 后显示的完整字幕快照
        case cea608.EventError:
            // 可按 event.Error.Kind 区分奇校验错误与非法字节对
        }
    }
}

fmt.Println(d.Displayed())
```

若只关心流结束时的显示字幕：

```go
text := cea608.DecodeText(ccBytes)
```

## API

- `NewDecoder` / `Decode` / `DecodeText`：创建解码器及一次性便捷入口。
- `(*Decoder).Feed`：流式解码，返回本批完成的 `Event`。
- `(*Decoder).Displayed` / `DisplayedCaption`：读取当前显示字幕。
- `(*Decoder).Stats`：读取字节、字节对、校验错误、丢弃对和字幕计数。
- `ValidParity` / `StripParity` / `AddOddParity`：奇校验基础函数。

字幕输出支持基础字符、特殊字符、扩展字符、PAC 定位，以及 RCL、RDC、
RU2/3/4、CR、EDM、ENM、EOC 等常用控制码。

## 验证

```sh
go test ./...
go test -race ./...
go vet ./...
```

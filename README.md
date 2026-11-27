# ASS transform → WebVTT 静态分段

纯 Go 标准库实现。它读取 ASS Dialogue，解析 `\t(...)` 时间区间，把连续的
缩放、旋转和位置变化拆成目标格式可承载的静态 WebVTT cue。动画结束状态不会
被提前应用到整句。

## 行为

- 以 ASS 事件起点为时间零点解析 `\t(t1,t2,tags)`，同时支持
  `\t(t1,t2,accel,tags)` 和省略时间的 `\t(tags)`。
- `\fscx`、`\fscy`、`\frx`、`\fry`、`\frz`、`\pos`、`\org`、`\an`
  和 `\move` 都进入时间线求值。
- 所有 transform 起止点都会成为分段边界；连续转场再按
  `Options.SampleInterval`（默认 40ms）采样。
- 每个静态分段有独立的起止时间。位置写成 WebVTT cue setting，缩放和旋转
  写成该分段专属的 `::cue(.class)` CSS transform。
- `\t` 参数按括号深度解析，逗号不会被 `\clip(...)` 等参数误切；嵌套
  `\t` 的起止区间会分别求值。
- 文本中的 ASS 换行 `\N` / `\n` 转为 WebVTT 文本换行，`\h` 转为空格，
  普通文本会做 WebVTT 转义。

首段始终使用事件开始时的状态。例如：

```text
{\fscx100\frz0\pos(320,180)\t(0,1000,\fscx200\frz90\pos(640,360))}Hi
```

会生成 `100% / 0° / (320,180)`、若干中间状态，以及
`200% / 90° / (640,360)` 的独立 cue，而不是把最终状态套到整句。

## 使用

```bash
go run ./cmd/ass2vtt -o output.vtt input.ass
go run ./cmd/ass2vtt -sample 50ms input.ass
cat input.ass | go run ./cmd/ass2vtt
```

作为库调用：

```go
script, err := ass.Parse(reader)
if err != nil {
    return err
}
vtt, err := script.WebVTT(ass.Options{SampleInterval: 40 * time.Millisecond})
```

也可以直接使用 `ass.Convert(reader, options)`。`Script.Segments` 返回未渲染的
静态分段，便于测试或生成其他目标格式。

## 验证

```bash
go test -race ./...
go vet ./...
```

测试覆盖格式行/逗号文本、CSS 与 VTT 时间戳、多个 transform、嵌套 `\t`、
带逗号的标签参数，以及动画首段与结束段状态分离。

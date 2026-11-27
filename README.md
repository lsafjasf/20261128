# subtitlelayout

一个只依赖 Go 标准库的最小子字幕行布局器。布局只使用调用方提供的字符宽度表，不猜测字体度量，也不依赖外部排版库。

## 快速开始

```go
widths := subtitlelayout.WidthTable{
    'H': 0.6, 'i': 0.3, '你': 1.0, '好': 1.0, '，': 1.0,
}
lines, err := subtitlelayout.Layout(
    "Hi你好，",
    widths,
    subtitlelayout.Options{MaxWidth: 2.6, MarginLeft: 0.1, MarginRight: 0.1},
)
if err != nil {
    // 按错误类型处理
}
for _, line := range lines {
    // line.Text 是这一行的原始字符，line.Width 是不含边距的实际宽度
}
```

`Options.MaxWidth` 是包含左右边距的总宽度；可用于排版的宽度为：

```text
MaxWidth - MarginLeft - MarginRight
```

也可以复用已校验的布局器：

```go
layouter, err := subtitlelayout.NewLayouter(widths, opts)
lines, err := layouter.Layout(text)
```

## 行为

- 全角、半角和任意比例字体都完全由 `WidthTable` 的宽度决定，不按 rune 数量估算。
- 组合字符与前一基字符组成不可拆分单元。`Mn`/`Me` 类组合字符宽度为 0；`Mc` 类若表中有宽度则使用该宽度。
- 保留 `\n`、`\r\n` 和单独的 `\r` 硬换行，包括空行与末尾换行后的空行。
- 使用贪心最长可行行和标点禁则：行尾不能是 `(`、`「`、`“` 等开标点，行首不能是 `,`、`。`、`」`、`”` 等闭标点。
- 所有字符和标点都会保留；无法满足标点禁则时不会静默溢出。
- 每个自动换行后的行宽不超过可用内容宽度（仅有 1e-9 量级的浮点比较余量）。

## 错误

所有错误均可通过 `errors.Is` 判断：

| 哨兵错误 | 场景 |
| --- | --- |
| `ErrUnknownGlyph` | 基字符不在宽度表中 |
| `ErrNegativeMargin` | 边距为负数、`NaN` 或无穷 |
| `ErrTextDoesNotFit` | 单个字素或标点禁则形成的不可拆分组超过可用宽度 |
| `ErrInvalidWidth` | 最大宽度或字形宽度非法 |
| `ErrInvalidUTF8` | 输入包含非法 UTF-8 |

错误消息带有逻辑行号、字符码点和字节偏移，便于定位字幕源文本中的问题。

## 测试

测试不依赖固定的大样本文件。`generated_test.go` 使用确定性随机种子生成混合中英文、全角/半角、组合字符、硬换行和标点的样本，并验证：

- 布局前后字符内容完全一致；
- 每行实际宽度与宽度表重新计算的结果一致；
- 行宽不超过上限；
- 组合字符没有脱离基字符；
- 自动断行处没有违反开/闭标点禁则。

```sh
go test ./...
go vet ./...
```

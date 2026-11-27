# TTML Ruby 字幕排版

这个仓库实现了一个零第三方依赖的 Go 排版器，用来处理带 ruby
注音的中文/日文 TTML 字幕。正文行盒和注音行盒分别布局：ruby
只改变注释盒的位置，不参与正文 `tts:lineHeight` 的计算，因此不会把下一行正文挤走。

## 快速开始

```bash
go test ./...
go run ./cmd/ttml-layout -in subtitle.ttml
```

从标准输入读取并输出紧凑 JSON：

```bash
cat subtitle.ttml | go run ./cmd/ttml-layout -compact
```

默认按 DejaVu Sans、Droid Sans Fallback 等常见系统字体建立回退链。也可以通过
`-font` 指定字体，重复该参数或使用逗号即可组成回退链：

```bash
go run ./cmd/ttml-layout \
  -in subtitle.ttml \
  -font /path/to/Latin.ttf,/path/to/CJK.ttf \
  -out layout.json
```

CLI 参数：

- `-in` / `-out`：输入/输出路径，`-` 表示 stdin/stdout。
- `-font`：TTF/OTF/TTC 字体，按给定顺序回退。
- `-width` / `-height`：覆盖 TTML 根区域尺寸。
- `-region` / `-paragraph`：选择区域和从 0 开始的段落序号。
- `-padding-*`、`-ruby-gap`、`-ruby-scale`：调整内边距与注音几何。
- `-fallback`：字体缺字时用于测宽的替代字符，默认 `U+FFFD`。
- `-compact`：输出无缩进 JSON。

## 输出

坐标单位是 TTML 像素。`region` 是 TTML 区域矩形，`contentBounds`
是正文盒和所有注音盒的并集。注音可以伸到 `region` 之外，这一点会明确反映在
`contentBounds` 中，而不是通过改变正文基线来“塞回”区域。

```json
{
  "region": {"x": 40, "y": 260, "width": 560, "height": 80},
  "contentBounds": {"x": 240, "y": 221.6, "width": 160, "height": 118.4},
  "lines": [
    {
      "text": "漢字東京",
      "box": {"x": 256, "y": 260, "width": 128, "height": 40},
      "baseline": 291.078125,
      "ruby": [
        {
          "layer": 0,
          "text": "かん",
          "box": {"x": 256, "y": 240.8, "width": 32, "height": 19.2},
          "baseline": 255.9390625
        },
        {
          "layer": 1,
          "text": "han",
          "box": {"x": 256.957, "y": 221.6, "width": 30.086, "height": 19.2},
          "baseline": 236.7390625
        }
      ]
    }
  ]
}
```

每一行 `lines[].baseline` 是正文基线。`layer: 0` 是最靠近正文的注音层，
更大的层号继续向正文上方（`rubyPosition=before`）或下方堆叠。
`missing` 中记录缺字诊断，`line` 从 1 开始，`layer` 为 `base` 或
`ruby[n]`。

## 排版规则

- 正文行盒只使用基础字体的 ascent/descent、字号和 `tts:lineHeight`。
  注音层无论有一层还是两层，都不会改变正文行盒高度或基线间距。
- 每个注音层拥有独立的行盒、宽度和基线。默认注音字号是正文的 50%，
  层高为 `rubyFontSize * 1.2`。
- 换行以 Unicode 字素簇为单位，中文按字断行，拉丁文本优先按词断行。
- 一个 ruby 组放不下整行时，会按基础字形的实际宽度拆分。每一层的注音文本
  会按相同比例切成连续片段，随对应基础字进入下一行，不丢字、不重排。
- 缺字通过字体回退链解决；回退链也没有字形时使用替代字符测量宽度，并在
  `missing` 中输出原始码位和位置。
- 支持 `before`/`after` ruby 位置、`start`/`center`/`end` 对齐，以及
  `tts:fontSize`、`tts:lineHeight`、`tts:textAlign`、`tts:displayAlign`、
  `tts:padding` 和区域/段落样式引用。

支持的 ruby 形式：

```xml
<ruby><rb>漢</rb><rt>かん</rt></ruby>
<ruby><rb>漢</rb><rt>かん</rt><rt>han</rt></ruby>
<ruby><rb><ruby><rb>漢</rb><rt>かん</rt></ruby></rb><rt>han</rt></ruby>
```

后两种都会产生两层注释；直接子元素 `rt` 依次作为第 0、1 层，嵌套形式中
内层 ruby 是第 0 层，外层是第 1 层。

同时兼容 TTML2 的 span 写法：

```xml
<span tts:ruby="container">
  <span tts:ruby="base">漢</span>
  <span tts:ruby="text">かん</span>
  <span tts:ruby="text">han</span>
</span>
```

## 测试素材

测试素材不是手写 XML，而是由
[`internal/testfixture`](internal/testfixture/fixture.go) 在 Go 中生成：

- 嵌套双层注音和直接双 `rt` 注音；
- `tts:ruby="container/base/text"` 的 span 注音；
- 超过行宽、需要跨界拆分的多字 ruby；
- 正文和注音都包含字体不支持字符的缺字场景。

测试直接在内存中调用生成器。需要把素材落盘时：

```bash
go generate ./internal/testfixture
```

生成结果位于 `testdata/generated/`，也可以直接运行：

```bash
go run ./cmd/ttml-fixture -out testdata/generated
```

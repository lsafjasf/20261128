# imscfit

IMSC/TTML 字幕在「只保留文字和时间」的转换流程之外，重建 region 布局信息并
在不同画幅下检查可读性的命令行工具。**仅使用 Go 标准库，无第三方依赖。**

## 解决的问题

IMSC 字幕依赖 `tts:padding`、固定 region（`tts:origin`/`tts:extent`）以及
`tts:displayAlign`、`tts:overflow` 维持可读距离。转换时若只保留文字和时间，
这些约束全部丢失。`imscfit` 会：

1. 解析 `region`、`style`（含引用链）、`padding`、`displayAlign`、
   `overflow`、`wrapOption`、`fontSize`、`lineHeight`。
2. 在一个或多个目标画幅上把 `px`/`c`/`em`/`%` 长度统一解析成像素，
   重建 region 盒模型与 padding 后的内容盒（实际可用行宽）。
3. 用贪心断行（支持 `<br/>`、CJK 任意断行、`noWrap`）重排每条 cue，
   报告行数、实际占用宽高、溢出量、溢出方向与被裁剪行数。
4. 无法保持时给出两类具体候选：
   - **缩短文本**：带省略号的替换文案，以及它的行数/占用尺寸；
   - **扩大区域**：可直接回填的 `tts:origin`/`tts:extent` 像素值，
     并按 `displayAlign` 决定锚点；超出画布时明确提示只能缩短文本。

## 用法

```bash
# 默认画幅：16:9、4:3、2.39:1、9:16（高度取文档 tts:extent 或 1080）
go run ./cmd/imscfit testdata/sample.ttml

# 自定义画幅
go run ./cmd/imscfit --canvas hd=1920x1080,portrait=1080x1920 file.ttml

# 机器可读 JSON
go run ./cmd/imscfit --json file.ttml
```

## 状态含义

- `ok`：文本落在 padding 后的内容盒内。
- `clipped`：`tts:overflow="hidden"`，超出的行被 region 裁剪。
- `spilling`：`tts:overflow="visible"`（默认），文本溢出 region 边界，
  报告按 `displayAlign` 计算的上/下溢出像素。
- `no-region`：cue 引用了不存在的 region。

## 长度单位解析

- `px`：直接使用。
- `c`：按 `ttp:cellResolution`（默认 32x15）映射到当前画幅的 cell 像素。
- `em`：相对当前字号；span 内的 `em` 相对父级有效字号。
- `%`：region 的 origin/extent 相对根容器；padding 百分比相对 region 自身。

## 已知近似

字符宽度是基于 Unicode 区间的启发式估算（CJK/全角 1em、拉丁 0.55em 等），
不是字体 shaping 的精确结果，用于工程上的行宽/溢出判断而非像素级渲染。

## 测试

```bash
go test ./...
```

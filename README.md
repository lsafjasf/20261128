# 20261128

Repository initialized for 20261128.

## imscconv

IMSC image-profile 转换器，解决“一份 IMSC 中多个 cue 重复引用同一图片、
旧转换器按 cue 复制字节且区域定义不一致”的问题。

### 核心模型

- 资源哈希表（实现见 `imsc/resource.go`）
  - 每个图片按解码字节的 `sha256` 内容寻址，cue 只保存资源 key，不再复制字节。
  - 同一内容经不同 URI 到达会合并到同一条记录，全部 URI 记录在 `Resource.URIs`。
  - 同一 URI 出现不同内容时**阻止合并**：首条绑定保持不变，冲突写入
    `Registry.Conflicts()`，两份字节各自保留独立 key。
  - 外部 URI（字节未内嵌）无法按哈希合并，使用 `ext:<uri>` 独立键。
- 区域引用 `imsc/region.go`
  - 区域按结构哈希（origin/extent/displayAlign/textAlign/其余属性）去重，
    结构相同的多个 xml:id 合并为别名（`Region.Aliases`），cue 引用经别名解析。
  - 引用计数记录每个区域被多少 cue 使用；默认区域 `defaultRegion` 终止回退链。
- 回退链
  - 资源：`Registry.SetFallback/Resolve/Chain`，缺失时逐级回退并检测环。
  - 区域：`Region.FallbackID` + `RegionTable.Resolve/Chain`，未知区域落到默认区域。
  - 回退关系通过 `conv:`（`urn:imscconv:v1`）命名空间属性随文档持久化。

### 导出后再读取

`Document.Export` 只输出一份 `smpte:image`（或 data URI 模式），cue 通过
片段 URI/背景图属性引用；区域别名写入 `conv:regionAliases`。对导出结果再次
`Parse` 后，资源 key、区域别名、资源与区域回退链全部恢复，测试见
`imsc/document_test.go` 与 `imsc/fallback_test.go`。

### 用法

```sh
# 基础转换（默认 embedded：每个唯一图片只输出一个 smpte:image）
go run ./cmd/imscconv -in testdata/sample.xml -out out.xml

# 导出并立即重新解析，打印引用/冲突统计
go run ./cmd/imscconv -in testdata/sample.xml -verify -out out.xml

# data URI 模式（相同内容重新导入后仍合并为一个资源）
go run ./cmd/imscconv -in testdata/sample.xml -mode datauri -out out.xml
```

### 测试

```sh
go test ./...
go vet ./...
```

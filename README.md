# 20261128

Repository initialized for 20261128.

## wavinfo

读写 WAV（RIFF）文件中 `LIST`/`INFO` 标签的 Go 库与命令行工具。

### 核心保证

- **数据部不变**：`data`、`fmt ` 等所有非 INFO 子块按原始字节（含头、载荷、RIFF 填充字节）原样写回；只有 `LIST INFO` 块会重建，外层 RIFF 尺寸自动重算。
- **可识别 INFO 子块解析**：按 RIFF 规范解析 `IART`/`INAM`/`IPRD` 等标准字段，正确处理 NUL 结尾与奇数长度子块的字对齐填充。
- **未知字段保留**：无法识别的四字节 ID（含二进制载荷）原样保留，序列化时排在已知字段之后并保持解析时的相对顺序。
- **字段权威顺序**：标准字段严格按 RIFF 规范文档顺序（见 `CanonicalOrder()` / `fields.go`）输出。
- **项目字段映射与文件名冲突仲裁**：`title`、`album` 等项目字段名经 `ResolveField` 映射到 INFO ID；项目值与文件名解析值冲突时由 `Preference` 决定胜者（默认保留文件内嵌标签），冲突会在结果中报告。

### 库用法

```go
f, _ := wavinfo.Parse(reader)
f.Info().Set("IART", "Artist Name")
f.Info().Delete("ICMT")
out, _ := f.Bytes() // 仅 INFO 块变化，音频数据字节不变

res, _ := f.ApplyProjectFields(
    map[string]string{"title": "Project Title"},
    wavinfo.ParseFileName("Artist - Song.wav"),
    wavinfo.PreferEmbedded,
)
// res.Conflicted 记录内嵌值与外部值的分歧及最终胜者
```

### 命令行

```sh
go build ./cmd/wavinfo
wavinfo fields                  # 列出权威字段顺序
wavinfo inspect [-json] FILE    # 查看标签（~ 标记未知字段）
wavinfo get FILE FIELD
wavinfo set FILE FIELD VALUE
wavinfo delete FILE FIELD
wavinfo [-prefer embedded|project|filename] apply FILE < metadata.json
```

`apply` 从标准输入读取 JSON（键为 INFO ID 或项目字段名），原地改写文件（临时文件 + 原子 rename），并报告已应用字段、冲突与无法解析的键。

### 测试

```sh
go test ./... -race -cover
```

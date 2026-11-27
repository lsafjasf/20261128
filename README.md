# 20261128

Repository initialized for 20261128.

## LTO 媒体交接清单工具

纯 Go 标准库实现的媒体目录清单与验证工具，详见：

- `docs/MANIFEST.md`：清单结构、配对与验证语义。
- `docs/USAGE.md`：命令行用法与演示步骤。
- `cmd/lto-manifest/`：CLI 入口。
- `internal/manifest/`：扫描、依赖分析、重复检测与重排验证。

快速开始：

```sh
go test ./...
go build -o lto-manifest ./cmd/lto-manifest
lto-manifest scan -root <媒体目录> -out manifest.json -budget 12TB
lto-manifest verify -root <重排后目录> -manifest manifest.json -expect-colocation
```

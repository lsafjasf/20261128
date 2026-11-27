# 20261128

Repository initialized for 20261128.

## sidecar-audit

比较一组母版的 checksum sidecar(同名但内容可能不同),建立文件身份与证据链,
裁决是否允许发布。核心逻辑全部在 Go 中:

- `internal/algo` — 受支持算法注册表(md5/sha1/sha224/sha256/sha384/sha512,含 `sha-256` 等别名)
- `internal/sidecar` — sidecar 解析:GNU sum 行格式(扩展名或摘要长度推断算法)与 JSON 清单
- `internal/audit` — 身份聚合、同名冲突检测、载荷重算核验、报告渲染
- `cmd/sidecar-audit` — CLI

### 用法

```sh
go build ./cmd/sidecar-audit
./sidecar-audit [-payload DIR] [-json] [-strict] <sidecar...>
```

- 同名(规范化路径相同)且同算法但摘要不同 → **同名冲突,阻断发布**(退出码 1)
- 提供 `-payload` 时对每条断言重算实际文件,断言与载荷矛盾同样阻断
- 未知算法或非法摘要 → 降级为**不可判定项**,默认不阻断;`-strict` 下也阻断
- 每条断言保留出处(sidecar 文件、行号、原始行)与核验状态,构成证据链;`-json` 输出完整机器可读报告

示例见 `examples/`(三份母版 sidecar + 载荷目录)。

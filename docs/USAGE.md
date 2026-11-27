# 使用说明

## 构建

```sh
go build -o lto-manifest ./cmd/lto-manifest
```

## 生成清单

```sh
lto-manifest scan -root /path/to/media -out manifest.json -budget 12TB -fail-on-budget
```

- `-budget` 支持 `B/KB/MB/GB/TB/PB`（十进制，与 LTO 厂商容量口径一致）。
- `-exclude rel/path` 可重复，按相对路径跳过子树或文件。
- 不加 `-fail-on-budget` 时超预算只报告、不返回非零退出码。

## 重排后验证

```sh
# 容忍移动/改名，按内容哈希核对
lto-manifest verify -root /path/to/reorganized -manifest manifest.json

# 强制 sidecar 与媒体仍在同一目录，并导出 JSON 报告
lto-manifest verify -root /path/to/reorganized -manifest manifest.json \
  -expect-colocation -json verify-report.json

# 归档封箱前的严格核对：任何路径变化都失败
lto-manifest verify -root /path/to/final -manifest manifest.json -strict
```

## 退出码

- `0`：验证通过（或扫描成功且未触发预算失败）。
- `1`：发现缺失、损坏、多余文件、sidecar 分离或预算超限。
- `2`：命令行参数错误。

## 演示数据

`demo/` 内置一组含重复媒体与同名 sidecar 的样例：

- `demo/media/`：原始媒体树。
- `demo/media-reorg/`：重排并改名后的目录。
- `demo/out/manifest.json`：扫描清单。
- `demo/out/verify-report.json`：重排后的验证报告。

复现：

```sh
go build -o demo/lto-manifest ./cmd/lto-manifest
(cd demo && ./lto-manifest scan -root media -out out/manifest.json -budget 100B)
(cd demo && ./lto-manifest verify -root media-reorg -manifest out/manifest.json \
  -ignore-extras -expect-colocation -json out/verify-report.json)
```

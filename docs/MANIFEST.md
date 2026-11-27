# LTO 交接清单设计

## 设计目标

- 交接给 LTO 归档系统前，为媒体目录生成可审计清单。
- 清单内只允许出现相对路径，目录整体迁移或内部重排后仍可验证。
- 仅使用 Go 标准库，无外部依赖，可离线交叉编译。

## 路径规则

- 所有 `path` 字段均为相对扫描根的 POSIX 风格路径（正斜杠），顶层 `root` 固定为 `"."`。
- 扫描时通过 `filepath.Rel` + `filepath.Clean` 净化，拒绝绝对路径与任何 `..` 越界路径。
- 符号链接与非常规文件不归档，只在 `issues` 中记录，避免链接目标丢失后产生死档。

## 清单字段（manifest.json）

- `entries[]`：相对路径、类别（`media`/`sidecar`/`other`）、媒体子类、MIME、扩展名、字节大小、修改时间、SHA-256 内容哈希。
- `relationships[]`：sidecar → media 依赖，同时保存双方相对路径与哈希；路径变化后仍可凭哈希重建关系。
- `duplicates[]`：按 SHA-256 聚合的同内容文件组及可回收字节 `wasted`。
- `issues[]`：孤儿 sidecar、歧义配对、符号链接、读取/哈希错误等结构问题。
- `budget`：总量、去重后唯一量、可回收量、预算比、是否超限。

## Sidecar 配对规则

仅在同一目录内配对，按以下优先级：

1. sidecar 去扩展名后等于媒体完整文件名，例如 `clip.mp4.srt` → `clip.mp4`。
2. sidecar 与媒体共享去扩展名的 stem，例如 `photo.xmp` → `photo.CR2`。

零候选记 `orphan_sidecar`，多候选记 `ambiguous_sidecar`，不做猜测。

## 验证语义

- 严格模式（`-strict`）：清单路径必须原地保留，移动即失败。
- 重排容忍模式：按 SHA-256 在当前树中定位文件，输出 `rehomed`（旧路径 → 新路径）。
- 重复内容按多重集一一匹配，N 个相同清单条目匹配 N 个相同现存文件，绝不复用同一文件。
- sidecar 关系成员优先匹配同目录落点；`-expect-colocation` 进一步强制媒体与 sidecar 仍同目录。
- 找不到原哈希记 `missing`；路径仍在但哈希变化记 `corrupt`；清单外文件记 `extra`。
- 退出码：全部通过为 0，存在失败项为 1，参数错误为 2。

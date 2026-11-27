# 20261128

## clipmerge — 素材身份冲突检测与修复工具（纯 Go，仅标准库）

两个拍摄日使用相同 bin 名称与片段编号，合并工程后引用串线。本工具按三层身份
扫描合并工程，输出冲突组、确定性重命名计划，并对锁定到错误文件的片段生成显式回滚条目。

### 三层身份模型

1. 命名空间层：`bin + clipNo`（编辑面身份，跨拍摄日易撞名）。
2. 媒体 UID 层：`media.uid`（NLE 内部唯一标识，跨工程合并可能被复用）。
3. 内容哈希层：`sha256(file)`（文件内容身份，用于识别同内容别名与内容被调换）。

### 冲突组类型

| Kind | 层级 | 严重度 | 含义 |
| --- | --- | --- | --- |
| `NAMESPACE_COLLISION` | 命名空间 | critical | 同 `bin/clipNo` 被不同拍摄日使用 |
| `FULLKEY_DUPLICATE` | 命名空间 | critical | 同一拍摄日内 `bin/clipNo` 重复 |
| `UID_REUSE` | UID | critical | 同一 UID 被不同拍摄日的片段锁定（串线主因） |
| `UID_SPLIT` | UID | warning | 同日多个片段锁定同一 UID |
| `HASH_ALIAS` | 哈希 | warning | 不同 UID 内容完全相同（真重复） |
| `HASH_DRIFT` | 哈希 | critical | 注册表哈希与磁盘实际哈希不符（文件被调换） |
| `LOCK_DANGLING` | UID | critical | 片段锁定的 UID 在合并注册表中不存在 |
| `WRONG_LOCK` | 三层联动 | critical | 片段锁定到错误文件，已解析出回滚目标 |
| `ROLLBACK_BLOCKED` | 三层联动 | critical | 确认锁错但无唯一同日目标，禁止自动回滚 |

### 回滚目标解析顺序

1. `expectedUID` 存在且属于本拍摄日、且不等于当前锁定 UID —— 标记 `expectedUID`。
2. 用 `lockedHash` 在同日素材中反查，命中且唯一、且不是当前 UID —— 标记 `hashLookup`。
3. 均不满足 —— `blocked`，只出人工处理条目，`apply` 不会改动该片段。

### 重命名规则（确定性）

- 跨日撞名：`clipNo -> clipNo_<shootDay>`；同日再撞时追加 8 位片段 ID 哈希短码。
- 同日重复：保留排序首个编号，其余追加 `_dup<8位短码>`。

### 用法

```bash
# 分析（--media-root 启用磁盘哈希校验；--fail-on-conflict 时存在 critical 退出码为 2）
go run ./cmd/clipmerge analyze \
  -m testdata/merged-manifest.json \
  --media-root testdata/media \
  --report report.json --plan plan.json --fail-on-conflict

# 应用计划：生成修正后的新清单，绝不原地改写输入文件
go run ./cmd/clipmerge apply \
  -m testdata/merged-manifest.json -p plan.json -o corrected.json
```

`apply` 只重写片段的 `bin/clipNo` 与锁定 UID/哈希；被 `blocked` 的片段保持原样并在
stderr 告警。对 `corrected.json` 再跑一次 `analyze` 即可验证可自动修复的冲突是否清零。

### 输入清单格式

见 `testdata/merged-manifest.json`：顶层为扁平的 `media`（UID、路径、声明哈希、
拍摄日）与 `clips`（bin、clipNo、当前锁定 UID/哈希、可选 `expectedUID`）。

### 测试

```bash
go test ./...
go vet ./...
```

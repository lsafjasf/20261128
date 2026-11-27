# 增量 Autosave 与恢复链设计

针对“应用连续崩溃后，恢复点比磁盘上的工程还旧，导致约一小时操作丢失”的问题，
本模块用**基准快照 + 增量快照链 + 原子清单**替代“单个恢复文件覆盖写”。

## 目录布局

```
recovery/
├── manifest.json            # 恢复链清单（原子写），记录最后成功写入
├── snap-000001-<id>.json    # base 快照（完整工程）
├── snap-000002-<id>.json    # incremental 快照（仅新增操作）
├── snap-000003-<id>.json
└── archive/                 # 压缩或损坏隔离的旧快照，保留可手动选择
```

## 核心机制

### 1. 操作序号（`project.Operation.Seq`）

- 操作日志只允许追加，下一个操作必须是 `LastSeq + 1`，出现跳号/重号直接返回
  `project.ErrSeqGap`，从源头杜绝链断裂后静默继续。
- 增量快照只保存 `seq > 链头 seq` 的操作，并再次校验首条操作接续链头。

### 2. 基准与增量

- `SaveBase` 写入完整工程的自包含快照，并将旧链整体移入 `archive/`。
- `AppendIncremental` 仅写入新操作，携带 `prev_id`/`parent_id` 指向前驱与基准。
- 默认每 10 个增量自动压缩为新基准（`WithBaseEvery` 可调，0 关闭）。
- `Auto` 在没有基准时建基准，之后追加增量。

### 3. 崩溃安全写入与“最后成功写入”

- 每个快照走 `临时文件 → fsync → 原子 rename → fsync 目录`，崩溃只会留下
  `*.tmp`，重开时自动清理，不会出现半个快照。
- 快照数据落盘并 fsync 之后才更新 `manifest.json`（同样原子写）。
- 清单中的 `last_success_seq` / `last_success_at` 即“最后成功写入”的权威记录。
- 若崩溃发生在“快照已写、清单未更新”之间，重开时 `reconcile` 会校验孤儿快照：
  合法且接续链头则采纳进清单；损坏或不接续则移入 `archive/` 隔离，绝不信任。
- 每个快照带 sha256 `checksum`，覆盖磁盘上的确切字节，追加空白/篡改均判定损坏。

### 4. 依赖媒体记录

- 工程通过 `project.MediaRef` 记录每个依赖媒体的路径、大小、内容哈希。
- 每个快照冗余保存当时的媒体清单。
- `recovery.FileVerifier` 在恢复前 stat 校验存在性与大小，可选校验内容哈希；
  缺失媒体以警告形式返回，**不阻塞**工程恢复（用户可重新链接素材）。

### 5. 恢复（失败安全，绝不覆盖原始工程）

- `recovery.Inspect` 重放整条链，逐链接校验结构、序号连续性和状态哈希，
  输出按时间倒序排列的候选快照列表，标记每个候选是否可达及媒体告警。
- 链在某一环断裂时，`Break` 记录断裂点，同时继续给出所有更早但仍可达的候选，
  供用户在 UI 中手动选择更近的可用快照。
- `recovery.Restore` 把所选快照重放为完整工程，写入**新文件**
  （`<工程名>.recovered-<时间>-<快照id>.json`，重名自动加序号），
  原始磁盘工程保持不动；返回结果中明确 `OriginalKept` 与媒体告警。
- 当所有快照都不可达时，不产生恢复文件，用户继续使用原始工程。

## 快速验证

```bash
go test ./...
go run ./cmd/autosave-demo demo --root /tmp/as-demo
go run ./cmd/autosave-demo inspect --recovery /tmp/as-demo/recovery \
  --disk /tmp/as-demo/project/edit.json
go run ./cmd/autosave-demo restore --recovery /tmp/as-demo/recovery \
  --candidate <快照ID> --disk /tmp/as-demo/project/edit.json --out /tmp/as-demo/recovered
```

演示场景：磁盘工程停留在 seq 2，而 autosave 链头已到 seq 3（恢复点更新）；
随后人为损坏 seq 3 快照并删除一个素材文件，巡检报告给出断裂点并推荐 seq 2
候选，恢复出的两个文件都不会覆盖原始工程。

## 包结构

- `project`：工程模型、追加式操作日志、媒体依赖、确定性状态哈希。
- `autosave`：快照存储、原子写、清单、崩溃对账与链压缩。
- `recovery`：链路巡检、媒体校验、候选列表与故障安全恢复。
- `internal/demo`：可复现事故的端到端场景。
- `cmd/autosave-demo`：`demo` / `inspect` / `restore` 命令行入口。

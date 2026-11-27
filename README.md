# batchout

一个只使用 Go 标准库实现的批处理输出事务层。目标是在同一输出目录里安全地处理：

- 旧成片和不属于本系统的人工文件；
- 中断后留下的 `.part` 或其他半成品；
- 多个新任务想写入同名结果的情况。

核心原则是：**只创建尚不存在的路径，从不覆盖、重命名或删除输出目录中的外部文件。**

## 目录布局

```text
output/
  old.mp4                         # 人工/旧文件，永远不碰
  old.mp4.part                    # 人工/旧半成品，永远不碰
  final.mp4                       # 第一个无冲突任务可使用原名
  final.8f14e45fceea.mp4          # 后续任务使用任务指纹冲突名
  .batch/
    journals/<task-id>-<attempt-id>.json
    locks/<task-id>.lock
    tmp/<task-id>/<attempt-id>/...
    records/<task-id>.json
```

只有 `.batch/` 内部属于本系统。日志和记录均采用“同目录临时文件 + fsync + rename +
目录 fsync”的方式原子替换。

## 任务身份

任务 ID 是以下内容的 SHA-256：

- 格式版本；
- 可选的 `Recipe`（建议放处理管线/工具版本）；
- 按调用顺序排列的输入逻辑名、输入大小、输入内容 SHA-256；
- 参数的规范 JSON（Go 的 map JSON 编码会稳定排序键）。

输入的物理路径会写入溯源记录，但不参与任务 ID，因此同一个输入文件搬家后仍是同一任务。
`ParamsDigest` 单独保存，便于快速比较参数集合。

同一个任务 ID 已提交时，`Run` 直接返回原结果而不重复执行。不同参数或不同输入内容会得到
不同任务 ID，即使它们请求的输出文件名相同。

## 任务锁

`.batch/locks/<task-id>.lock` 上使用 `flock(LOCK_EX | LOCK_NB)`：

- 同输入、同参数的任务跨进程只能执行一份；
- 锁随进程退出或崩溃自动释放，不会留下需要人工删除的“死锁文件”；
- 锁文件本身保留不删，避免“删除旧锁文件后两个进程锁住不同 inode”的竞态；
- 默认立即返回 `ErrTaskLocked`，`WithLockWait(d)` 可改为等待指定时间。

## 事务状态机

```text
staging -> prepared -> committing -> committed
    \          \
     +----------+-> aborted
```

- `staging`：生产者只在 `.batch/tmp/...` 写文件，最终目录尚不可见。
- `prepared`：生产者结束，所有暂存文件已 fsync 并计算大小、SHA-256、权限。
- `committing`：WAL 已持久化提交意图；这是第一个允许发布最终文件的时刻。
- `committed`：全部最终文件已发布，不可变溯源记录已写入。
- `aborted`：未进入提交阶段，暂存区可安全删除。

最终文件通过 `os.Link(staged, target)` 发布。硬链接创建是原子的，目标路径存在时必然失败，
因此不存在“先检查再 rename 覆盖”的窗口。成功后再 fsync 目标父目录。

## 冲突命名

依次尝试：

```text
report.mp4
report.<task-id 前 12 位>.mp4
report.<task-id 前 12 位>-2.mp4
...
```

目录层级保持不变，扩展名保留，过长文件名按 UTF-8 字符边界截断。目标存在时，普通流程通过
`os.SameFile` 判断它是否与暂存源为同一 inode，只有同一硬链接才视为“崩溃后已链接成功”。
即使人工文件内容完全相同，也会继续使用下一个候选名；仅当暂存源已经丢失、需要重建记录时，
才以大小和 SHA-256 作为兜底判断。人工文件、旧成片、`.part` 和符号链接都不会被覆盖或跟随。

## 崩溃恢复

恢复入口为 `Manager.Recover`，`Open` 默认自动执行；也可用 CLI：

```bash
go run ./cmd/batchout recover -output /data/output
```

| 崩溃时状态 | 恢复动作 |
| --- | --- |
| `staging` | 未发布任何文件，标记 `aborted` 并清理暂存区 |
| `prepared` | 未发布任何文件，回滚并清理 |
| `committing` | 逐文件幂等续做，补齐缺失输出、写溯源记录、标记 `committed` |
| `committed` | 保留记录；记录意外缺失时由日志重建，并清理残留暂存区 |

恢复会先拿全局恢复锁，再尝试任务锁。任务锁仍被活进程持有时跳过该任务，绝不删除正在生产的
暂存数据。没有日志引用的外部文件不会被扫描或清理。

## 溯源

`.batch/records/<task-id>.json` 保存任务 ID、尝试 ID、Recipe、输入列表（逻辑名、绝对路径、
大小、SHA-256）、参数、参数摘要，以及每个最终文件的原始名、实际路径、大小、SHA-256、权限。

```go
found, err := mgr.Lookup("final.8f14e45fceea.mp4")
fmt.Println(found.TaskID, found.Inputs, found.Params)
```

文件被人工改名后，可使用 `LookupDigest(sha256)` 继续定位来源。`Manifests()` 返回全部已提交
记录，适合生成审计清单。无法在记录中找到的路径就是未托管文件，而不是“未知成功输出”。

```bash
go run ./cmd/batchout lookup -output /data/output -file final.8f14e45fceea.mp4
go run ./cmd/batchout lookup -output /data/output -digest <sha256>
go run ./cmd/batchout list   -output /data/output
go run ./cmd/batchout verify -output /data/output
```

## 接入方式

生产者只接收暂存目录，不需要知道最终文件名冲突如何解决：

```go
mgr, err := batchout.Open("/data/output")
if err != nil {
    return err
}

result, err := mgr.Run(ctx, batchout.TaskSpec{
    Inputs: []batchout.Input{
        {Name: "source", Path: "/data/input/source.mov"},
    },
    Params: map[string]string{"codec": "h264", "crf": "18"},
    Recipe: "renderer-2026.11",
}, func(ctx context.Context, stage string) error {
    return render(ctx, "/data/input/source.mov", filepath.Join(stage, "final.mp4"))
})
```

`result.Outputs[i].Path` 是最终相对路径；`result.Outputs[i].Name` 是生产者请求的原始名。

## 一致性边界

- 单个文件从出现开始就是完整内容，不存在读者看到“正在写入的最终文件”的窗口。
- 多文件事务是**可恢复的分组提交**，不是文件系统层面的瞬时多文件可见性。若消费者要求
  “全部可见才算成功”，应通过 `Lookup`/`Manifests` 读取已提交记录后再消费该组文件。
- 暂存区和输出根目录必须位于同一文件系统，因为提交依赖硬链接原子性。不支持硬链接的文件
  系统会返回错误，而不是退化为可覆盖或不完整的提交。
- 任务锁当前在 Linux、macOS 和 BSD 上使用 `flock`；其他平台会明确返回不支持错误。
- 输入摘要是在任务开始前计算的。若外部进程在任务执行期间原地改写输入，需要由调度层保证
  输入不可变，或将输入复制到任务私有目录。

## 验证

```bash
go test -race ./...
go vet ./...
```

测试覆盖：正常提交与幂等重跑、人工文件/旧半成品保护、任务锁竞争、计划后出现的同名冲突、
提交中断续做、提交前回滚、符号链接保护。

# 20261128

Repository initialized for 20261128.

## 媒体身份模型（Go）

解决的问题：从同一母片段切出的两个 subclip 即使源区间完全相同，也必须是
两个独立身份；复制、改名、重新修剪、删除后，时间线实例的引用必须仍然正确。

### 三层身份

- `MasterMedia`（母媒体）：源资产，身份是不透明 ID，路径/名称只是可变属性。
- `Subclip`（子片段）：通过母媒体 ID + `TimeRange` 引用源区间，名称与标记
  （markers）独立可变；源区间相同也不会合并，每次 `CutSubclip` 都生成新 ID。
- `Instance`（时间线实例）：只通过 subclip 的稳定 ID 建立引用，与名称/区间解耦。

ID 由类型前缀 + 随机字节 + 单调序号构成（`medialib/types.go`），全生命周期
不变，从不根据名称或源区间重新生成。

### 关键不变量

- 身份只由 ID 决定：`(master, range)` 相同的两个 subclip 拥有不同 ID，互不合并。
- 改名（`Rename`）、改标记（`SetMarker`）只改属性，ID 与全部引用保持不变。
- 重新修剪（`Retrim`）修改区间但保留 subclip ID，所有 instance 继续指向它。
- 复制（`DuplicateSubclip`）产生新身份并深拷贝 markers，副本与原件的可变状态互不影响。
- 删除带引用保护：被引用时返回 `ErrInUse`；`cascade=true` 才级联清理依赖，
  防止悬空引用。任何非法区间返回 `ErrBadRange` / `ErrRangeOutOfBounds`。
- `VerifyIntegrity` 校验所有引用可解析、区间在母媒体时长内。

### 运行

```bash
go test ./...          # 7 个测试：身份独立、改名、重修剪、复制、删除保护、端到端
go run ./cmd/demo      # 场景演示
```

代码：`medialib/types.go`（ID 与类型）、`medialib/library.go`（操作与完整性校验）、
`medialib/*_test.go`（验证）、`cmd/demo/main.go`（演示）。

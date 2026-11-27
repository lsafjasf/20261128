# 带版本号的 Caption Session

直播字幕晚于语音约 2 秒，运营改一句会触发整屏滚动，历史观众与当前观众
看到的状态不一致。本包用「版本化追加日志 + 媒体时间戳 + 冻结水位线」
解决这三类问题。

## 核心模型

- `Segment`：一条字幕，按 `ID` 标识，锚定媒体时间轴 `[StartPTS, EndPTS]`。
- `Op`：一次状态变更，带全局单调递增的 `Version`，共四种：
  - `upsert`：ASR 临时片段插入或修订；
  - `replace`：运营对**单条**片段的局部文本替换，邻居片段的 ID、位置、
    版本都不变，因此不会整屏滚动；
  - `watermark`：提交水位线前移，所有 `EndPTS <= watermark` 的片段冻结；
  - `snapshot`：日志压缩后的物化基线。
- `State`：由 `Snapshot + Op 前缀` 通过唯一的确定性函数 `Apply` 物化。
  直播观众、晚加入观众、断线重连观众走同一条回放路径，相同版本前缀
  必然得到相同状态。

## 四项诉求如何落地

### 1. 延迟补偿（字幕晚语音 2 秒）

- `LatencyEstimator` 用 EMA 平滑「事件墙钟到达时间 − 语音 endPTS」，
  带漂移钳制过滤时钟毛刺；无样本时用 `WithAssumedDelay`（默认 2000ms）。
- `Playhead = wallTime - estimatedDelay`，把墙钟换算成「当前正在说的
  PTS」，恒定管线延迟被抵消。
- `Scheduler` 用最小堆按 `StartPTS` 缓冲事件，乱序/迟到的 ASR 事件仍按
  时间轴顺序释放；`Pump` 可由后台 ticker 驱动（`Run/Stop`），也可在测试
  中用固定时钟确定性调用。

### 2. 局部替换，不整体滚动

- `Session.Replace(id, newText)` 只产出一条 `replace` Op，仅改目标片段
  的 `Text`，顺序切片 `order` 与其他片段完全不动。
- Op 同时记录 `OldText/NewText`，审计与回放都可追溯。

### 3. 已显示内容不得被未来修订倒灌

- 每次 pump 调 `AdvanceWatermark(playhead)`，水位线只增不减。
- `Upsert`/`Replace` 对 `EndPTS <= watermark` 的片段一律返回
  `ErrSegmentFrozen`；调度队列里跨过水位线才到期的迟到事件在释放时被
  静默丢弃，不会注入历史时间轴。
- 因为冻结是时间轴语义而非客户端行为，所有观众对「哪句已锁定」的判断
  完全一致。

### 4. 断线重连

- `Subscribe()`：新观众立即收到当前 `snapshot` 事件，之后接收增量 Op。
- `SubscribeFrom(lastVersion)`：
  - 请求版本仍在保留日志内 → 只回放缺失 Op（增量恢复）；
  - 请求版本已被 `Compact` 压缩 → 先发 snapshot 再发增量；
  - 请求版本超过当前日志（异常客户端）→ snapshot + `ErrResync`。
- 慢消费者缓冲满后被标记 `Stalled` 并收到 `ErrResync`，不阻塞其他观众，
  客户端按 snapshot 重新同步即可。

## 主要 API

```go
sess := captionsession.NewSession()
sched := captionsession.NewScheduler(sess) // 可选：自动延迟补偿与释放

sched.Ingest(seg)                 // ASR 事件入队，按 PTS 释放
op, _ := sess.Upsert(seg)         // 或直接写入（测试/旁路）
op, _ = sess.Replace("u2", "修正文案")
op, moved, _ := sess.AdvanceWatermark(pts)

sub := sess.Subscribe()           // 直播/晚加入
sub2 := sess.SubscribeFrom(v)     // 断线重连
snap, _ := sess.Compact(64)       // 日志压缩
state, _ := captionsession.Apply(&snap, ops) // 确定性物化
```

## 测试

`go test -race ./...`，覆盖：

- 版本号单调、确定性回放与服务端状态严格一致；
- 冻结片段拒绝 ASR 修订与运营替换，迟到片段不注入；
- 局部替换不改变顺序；
- 延迟估计收敛、漂移钳制、播放头补偿；
- 调度器乱序输入按 PTS 排序释放；
- 增量重连、压缩后 snapshot 重同步、慢消费者 resync；
- `e2e_test.go` 串联完整直播场景。

## 设计取舍

- 水位线以 `EndPTS` 为冻结边界：一句话完整显示后才锁定，避免显示到一半
  的文本被永久冻结成错误内容。
- 订阅者采用非阻塞投递 + 显式 resync，而不是对慢客户端施加反压，防止
  一个卡顿观众拖垮整个会话。
- 日志压缩是显式操作（`Compact`），保留最近 N 条 Op，重连协议因此只有
  「增量」与「快照 + 增量」两种情况，客户端实现简单。

# 20261128

## 章节边界与字幕会话

解决的问题：播放器从第 8 章跳回第 2 章后，字幕仍显示旧章节最后一句。
实现把“章节边界”“字幕查询”“播放器会话状态”分开建模，任何跳转都从新的
绝对时间重建快照，旧字幕不会被顺带保留。

只使用 Go 标准库，包入口为 `github.com/lsafjasf/20261128/subtitle`。

### 边界规则

- `Range` 是半开区间 `[Start, End)`。时间正好等于边界时属于新章节，而不是
  刚刚结束的旧章节。
- 章节可以重叠。同一时间命中多个章节时，`ChaptersAt` 返回全部候选，绝不
  按 ID、顺序或时长自动挑一个。
- `Cue.ChapterID` 可把字幕绑定到章节；绑定字幕只在对应章节已解析后显示。
  空 `ChapterID` 是时间线级公共字幕。

### 会话状态

`Session` 同时保存位置、播放/暂停状态、人工选择与单调递增的 `Revision`。
每次 `Seek`、`SeekBy`、`Update`、`Play`、`Pause`、`Advance` 都会重新解析
当前位置：

- 向前跳转、向后跳转、外部时钟直接跳变都走同一套绝对位置解析。
- 暂停会冻结当前字幕；暂停期间 `Advance` 不移动时间。
- 恢复播放和任何跳转都会重新检查字幕区间。
- 无字幕区间得到 `Cues == nil`、`Subtitle == ""`，不会沿用上一句。
- `Snapshot` 是不可变拷贝；`IsCurrent` 可用 `Revision` 丢弃异步加载后已经
  过期的旧章节字幕结果。

### 歧义与人工判断

同时候选多个章节时，快照状态为 `ChapterAmbiguous`，`Chapter == nil`，但
`Candidates` 保留所有候选。此时只显示公共字幕。

```go
snap, _ := session.SelectChapter("chapter-2") // 仅在当前候选内有效
// snap.Status == subtitle.ChapterManual
```

`SelectChapter` 对非候选章节返回 `ErrNotCandidate` 且不改变状态。人工选择
会在所选章节仍是候选时保留，离开候选范围后自动清除；`ClearManual` 可主动
回到自动解析。

### 快速使用

```go
timeline, err := subtitle.NewTimeline(80*time.Second, chapters, cues)
if err != nil {
    return err
}

session := subtitle.NewSession(timeline)
session.Seek(75 * time.Second) // 第 8 章
snap := session.Seek(15 * time.Second) // 正确切到第 2 章字幕
```

运行：

```bash
go test ./...
go test -race ./...
go run ./cmd/demo
```

代码结构：`subtitle/types.go`（类型与错误）、`subtitle/timeline.go`（边界、
候选与字幕查询）、`subtitle/session.go`（跳转/暂停/人工选择状态机）、
`subtitle/*_test.go`（回归测试）、`cmd/demo/main.go`（场景演示）。

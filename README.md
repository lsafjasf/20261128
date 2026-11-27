# avsync — B 帧启用后切镜头音频空白的诊断与修复

纯 Go（仅标准库）实现。解析 MP4 时间表，按**解码顺序 / 显示顺序 /
音频样本位置**建立事件表，把切镜头处几十毫秒的音频空白归因到三类
根因之一，并做**定点修复**（绝不对整条音轨做全局延迟）。

## 用法

```sh
go build -o avsync .

# 诊断：打印事件表 + 根因判定
./avsync diag input.mp4

# 修复：重封装出修正文件，并自动复检
./avsync fix input.mp4 -out fixed.mp4

# 生成带指定缺陷的合成样本（用于复现/回归）
./avsync gen -defect reorder    -out reorder.mp4     # 重排等待
./avsync gen -defect elst       -out elst.mp4        # 编辑列表
./avsync gen -defect clockreset -out clockreset.mp4  # 时钟重置
./avsync gen -defect none       -out clean.mp4       # 无缺陷对照
```

合成样本的码流负载是占位字节（只保证容器与时间表合法）；诊断工具
本身可解析真实编码器产出的 MP4（已用 ffmpeg/libx264 + AAC 验证）。

## 事件表与判定逻辑

`diag` 输出三部分：视频解码序表（DTS/CTS/ctts 偏移/同步点）、音频
解码序表（PTS/帧长/步进）、以及每个切镜点 ±150ms 的**显示序**合并
窗口。判定看三个指标（`av/diagnose.go`）：

| 根因 | 事件表特征 | 修复 |
|---|---|---|
| 重排等待 | 音频 stts 时间线本身在每个切镜点出现 ≈ 重排延迟（max CTS−DTS）的洞，且随切镜数累积漂移；视频表干净 | 逐切镜点消除音频 PTS 跳变，恢复连续时间线 |
| 编辑列表 | 两条媒体时间线都连续，但视频 elst 把显示时间相对音频平移（逐段累积） | 重写 elst 为单条 `media_time = minCTS`（为 0 则删除 edts） |
| 时钟重置 | 视频 DTS 在每个切镜点超出正常帧步进，音频连续；偏移逐切镜累积 | 逐切镜点消除视频 DTS/CTS 跳变 |

判定顺序有讲究：洞在音频媒体时间线里 → 采集/编码侧（重排等待）；
洞只在视频时间线里 → 时钟重置；两条时间线都干净 → 只能是 elst。

## 为什么修复不是全局延迟音频

三类缺陷的偏移都**随切镜次数累积增长**（第 k 个切镜偏 k×80ms），
任何常量偏移最多修好一点。修复按切镜点逐个消除不连续：样本越靠后
修正量越大，首个样本保持不动（`TestReorderFixIsNotGlobalDelay` 与
`TestNonAudioDefectsKeepAudioUntouched` 有回归保障）。

## 结构

- `mp4/` — 最小 MP4 demuxer（stts/ctts/stss/stsc/stsz/stco/elst）与
  remuxer（按 DTS 交错写 mdat，重建全部时间表）
- `av/` — 事件表构建、根因分类、定点修复
- `gen/` — 三类缺陷的合成样本生成器
- `e2e_test.go` — 端到端回归：分类正确性、修复收敛、修复局部性、
  负载逐字节保留

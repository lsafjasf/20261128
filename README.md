# ghostdiag

逐帧比较**原始序列**、**降噪序列**和**运动轨迹**，定位时域降噪留下的拖影，并把真正的运动模糊、时域残影和编码块分开。只使用 Go 标准库。

它不内置 H.264/HEVC 解码器。可先解成 Y4M、headerless planar YUV，或直接提供 PNG/JPEG 帧目录。

```bash
ffmpeg -i original.mp4 -an -pix_fmt yuv420p -f yuv4mpegpipe original.y4m
ffmpeg -i denoised.mp4 -an -pix_fmt yuv420p -f yuv4mpegpipe denoised.y4m
```

解码只是预处理；分析和参数回归本身只依赖 Go 标准库。

## 判别方法

每个像素先建立三类互相独立的证据：

| 证据 | 方法 | 用途 |
| --- | --- | --- |
| 时域滞后 | 降噪残差 `D_t - O_t` 是否能由某帧历史 `O_{t-k} - O_t` 解释：`r = α·h`，且 `α>0` | 识别旧人物/旧背景被混进当前帧 |
| 真实运动模糊 | 在运动方向上估计原始帧强边的 10–90% 过渡宽度 | 原始画面本身已经模糊时，不把它误算成降噪拖影 |
| 编码块 | 比较 8×8 / 16×16 网格边界与块内部残差梯度 | 识别块边界集中的压缩/降噪块伪影 |

时域滞后只在原始画面最近发生过变化的区域计算，因此人物离开后的持续拖影也会继续被检查。静止背景上的普通降噪残差不会进入候选区域；如果降噪帧与历史帧的匹配显著好于当前帧，则判为 `temporal_ghost`。

真实运动模糊是源内容属性。它若与时域残影同时存在，会出现在 `secondary` 中，而不是被静默丢弃。

## 输入

### Y4M

```bash
go run ./cmd/ghostdiag \
  -original original.y4m \
  -denoised denoised.y4m \
  -trajectory trajectory.json \
  -output-format json \
  -output report.json
```

支持 8-bit `C420*`、`C422`、`C444`、`Cmono`，只分析 Y 平面。

### Headerless YUV

```bash
go run ./cmd/ghostdiag \
  -format raw -size 1920x1080 -pix-fmt yuv420p -fps 25 \
  -original original.yuv -denoised denoised.yuv \
  -output-format text
```

`-pix-fmt` 支持 `yuv420p`、`yuv422p`、`yuv444p`、`gray`。

### 图片序列

```bash
go run ./cmd/ghostdiag \
  -format images -fps 25 \
  -original frames/original -denoised frames/denoised \
  -trajectory trajectory.json
```

帧目录按自然文件名排序，例如 `frame-2.png` 在 `frame-10.png` 前。

## 运动轨迹

`-trajectory` 可选。文件是 JSON 数组或 `{"points":[...]}`：

```json
[
  {"frame": 0, "x": 20, "y": 25, "w": 26, "h": 46, "vx": 12, "vy": 0},
  {"frame": 1, "x": 32, "y": 25, "w": 26, "h": 46, "vx": 12, "vy": 0}
]
```

`vx/vy` 单位为像素/帧。未提供轨迹时，工具用相邻原始帧的变化区域和局部平移搜索自动估计；显式轨迹更适合多人、遮挡或相机运动素材。

## 输出

默认输出 JSON。最重要字段：

```json
{
  "first_ghost": {
    "frame": 2,
    "time_seconds": 0.08,
    "classification": "temporal_ghost",
    "region": {"x": 17, "y": 25, "w": 27, "h": 46, "area": 1242},
    "evidence": {
      "temporal_score": 0.937,
      "block_score": 0.0,
      "blur_width_px": 3.85
    }
  },
  "summary": {"first_ghost_frame": 2}
}
```

- `classification`：`temporal_ghost`、`motion_blur`、`coding_block`。
- `region`：该帧内最大且最可信的对应连通区域，坐标是 `x,y` 加宽高。
- `summary.first_ghost_frame`：按输入顺序检测到的第一个时域重影帧。
- `static_background_residual_mad`：排除最近运动轨迹后的静止背景降噪残差。
- `estimated_source_noise_mad`：同一静止背景上的原始帧时域噪声。
- `static_noise_reduction`：静止背景噪声下降比例，范围 `0..1`。

文本报告：

```bash
go run ./cmd/ghostdiag ... -output-format text
```

退出码：`0` 完成，`1` 处理失败，`2` 参数错误或启用 `-fail-on-ghost` 后检测到重影。

## 参数回归

每次报告的 `input.sha256` 覆盖所有原始帧和降噪帧的尺寸、序号和全部 Y 采样，不包含帧率或时间戳等元数据。参数扫描时，即使阈值不同，只要画面相同，指纹就必须一致。

```bash
go test -run 'TestParameterRegressionUsesSamePictures|TestAnalyzeFindsEarliestTemporalGhost' ./...
```

测试对同一组帧依次使用三套阈值，并断言三份报告使用相同的输入指纹。

## 可复现样例

```bash
go run ./testdata
go run ./cmd/ghostdiag \
  -original testdata/original.y4m \
  -denoised testdata/denoised.y4m \
  -trajectory testdata/trajectory.json \
  -output-format json -output artifacts/ghosting_report.json
go run ./cmd/ghostdiag \
  -original testdata/original.y4m \
  -denoised testdata/denoised.y4m \
  -trajectory testdata/trajectory.json \
  -output-format text -output artifacts/ghosting_summary.txt
```

样例是 160×96、25 fps、9 帧的确定性合成序列：

- 第 1 帧有 8×8 编码块残差，判为 `coding_block`。
- 第 2 帧首次出现旧人物位置残留，判为 `temporal_ghost`，区域 `x17,y25,w27,h46`。
- 原始人物边缘本身有约 3.8 px 的运动模糊，因此保留 `motion_blur` 次要证据。
- 静止背景的传感器噪声被清除，背景指标只统计最近没有运动经过的像素。

结果见 `artifacts/ghosting_report.json` 和 `artifacts/ghosting_summary.txt`。

## 限制

- 只分析亮度；色度拖影、纯色画面和极慢运动可能需要结合显式轨迹。
- 镜头运动、大面积遮挡和低于 `-motion-threshold` 的变化可能降低轨迹估计质量。
- `motion_blur` 是源画面属性，不表示降噪器错误；只有 `temporal_ghost` 才计入“最早重影”。
- 不做视频编解码，输入必须是已解码帧；参数回归不替代对具体编码器/降噪器的端到端测试。

## 验证

```bash
go test ./...
go test -race ./...
go vet ./...
```

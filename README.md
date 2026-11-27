# yuvdiag

用于审计 10-bit 4:2:0 YUV 解码与 YUV→RGB 转换的 Go 工具。它先检查原始码值，再做一次修正后的转换，因此不会把显示裁切误当成 YUV 越界，也不会把矩阵舍入或色度插值产生的假溢出算成真实像素。

## 检查项

- **10-bit 样本**：支持 `yuv420p10le`（低 10 位）和 `p010le`（高 10 位，低 6 位填充为零）。读取时不裁剪，`>1023` 的码值和 P010 非法填充位会单独计数。
- **合法范围**：10-bit limited 为 `Y 64..940`、`Cb/Cr 64..960`；full 为 `0..1023`。边界值本身合法。
- **矩阵舍入**：BT.601/709/2020 使用标准非恒定亮度系数；归一化后只做一次 nearest 舍入，最后才裁剪，避免截断造成的 ±1 偏移。
- **色度上采样**：4:2:0 使用整数权重的边缘钳制双线性插值，支持 `left`、`center`、`top`、`topleft`。权重是凸组合，因此不会在色度样本之间产生上下冲。
- **显示裁切**：RGB 超出显示范围只在最终量化阶段裁切；转换前的越界值会单独列出，不与 YUV 码值越界混淆。
- **来源归因**：提供参考帧时，同坐标源码值已越界记为 `input`，源合法而解码越界记为 `decode`；没有参考帧时，位深/填充异常可归为 `decode`，其余只报告为 `input_or_decode`，不猜测。

## 使用

```bash
go run ./cmd/yuvdiag \
  -input decoded.yuv -reference source.yuv \
  -width 1920 -height 1080 \
  -bit-depth 10 -pix-fmt yuv420p10le \
  -range limited -matrix bt709 -chroma left \
  -rgb-depth 8 -json -report report.json
```

`-reference` 是可选的解码前/源 YUV 帧；只有它才能严格区分输入与解码引入的越界。P010 输入使用 `-pix-fmt p010le`。可用 `-frame` 选择帧序号，`-y-stride`/`-c-stride` 指定带 padding 的行跨距，`-ppm out.ppm` 导出修正后的 RGB 帧。

`-range`、`-matrix` 和 `-chroma` 必须与流元数据（例如 ffprobe 的 `color_range`、`color_space`、`chroma_location`）一致；否则 limited/full 或色度相位选错本身就会制造假越界与边缘色差。

`-fail-on-violation` 会在存在真实码值越界或显示裁切时返回退出码 2，便于流水线使用。

## 样例结论

仓库里的 `testdata/` 是可复现的 16×8、10-bit limited BT.709 回归样本：

```bash
go run testdata/generate.go
go run ./cmd/yuvdiag \
  -input testdata/decoded_10bit.yuv \
  -reference testdata/reference_10bit.yuv \
  -width 16 -height 8 -json -report artifacts/yuv_report.json
```

结果见 `artifacts/yuv_report.json` 和 `artifacts/yuv_summary.txt`：

- 真实越界：`2 samples / 2 luma-grid pixels`，`Y(0,0)=63` 来自输入，`Y(1,0)=941` 来自解码。
- 这两个 ±1 码值在 8-bit 显示量化后没有造成裁切，所以播放器的颜色差异很小。
- 色度上采样过冲为 `0`。
- 另有 `2` 个像素由**合法** YUV 极端组合产生 Red 通道显示裁切；它们记为 `conversion_only`，不算 YUV 码值越界。

结论：样例中的“超范围”来自输入码值和解码码值，不是矩阵舍入或色度上采样；转换只产生了合法 YUV 组合的显示色域裁切。

## 验证

```bash
go test ./...
go vet ./...
```

测试覆盖 10-bit/P010 读取、limited/full 边界、矩阵中点舍入、双线性无过冲、真实像素计数、输入/解码归因、转换裁切分离和 CLI JSON 输出。

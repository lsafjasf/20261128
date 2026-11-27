# mp4safe

纯 Go 标准库实现的 ISO BMFF（MP4/MOV）安全归档改写器。它只删除顶层
`free`/`skip` padding，并只改写必要的 `stco`/`co64` 块偏移；媒体载荷与未知
box 均按原字节流式复制，不会经过重编码或重新序列化。

## 安全边界

| 类别 | 识别方式 | 处理 |
| --- | --- | --- |
| 媒体载荷 | `mdat` payload | 原字节复制；额外计算整个 payload 的 SHA-256 |
| 索引 | `stco`/`co64` 表项 | 仅将受 padding 删除影响的绝对块偏移减去删除量 |
| 未知扩展 | 未识别的 4CC、私有 box、`uuid` box | 不解析、不重建、不修改；原字节复制并比对 SHA-256 |
| 可删除 padding | 顶层 `free`/`skip` | 整箱删除；嵌套 padding 不处理 |
| 其他结构 | 已知 box/容器 | 原字节复制；只解析 `moov/trak/mdia/minf/stbl` 索引链 |

`stco`/`co64` 以外的字节不做重建。删除 padding 后，所有轨道样本范围仍必须落在
`mdat` payload 内，否则改写直接失败。

## 验证模型

落盘采用同目录临时文件，提交前重新解析输出并完成三组严格比对：

1. **逐样本哈希**：按 `stsc`、`stsz`/`stz2`、`stco`/`co64` 展开每个音视频样本，
   对样本字节计算 SHA-256；轨道摘要为“样本大小 + 样本哈希”的顺序聚合哈希。
2. **媒体载荷哈希**：逐个比对 `mdat` payload 的 SHA-256。
3. **未知块哈希**：逐个比对未知 box 的完整原始字节、类型、`uuid` user type、
   路径与长度。

只有三组摘要全部一致且输出不再含顶层 padding 时，临时文件才原子替换目标文件。
输入同时受保护的检查还包括：box 边界不越界、索引样本数一致、样本范围位于
`mdat` 内、轨道 ID 不重复。

## 命令行

```bash
# 查看 box 边界、索引、样本摘要与未知块摘要
go run ./cmd/mp4safe inspect testdata/av.mp4
go run ./cmd/mp4safe inspect -json testdata/av.mp4

# 安全删除顶层 padding，并生成可审计报告
go run ./cmd/mp4safe strip \
  -in archived.mp4 -out cleaned.mp4 -report report.json

# 独立复核两个文件的样本、mdat 和未知块哈希
go run ./cmd/mp4safe verify -before archived.mp4 -after cleaned.mp4
```

`strip` 不会原地破坏输入：即使 `-in` 与 `-out` 相同，也会先写临时文件、复验成功
后再替换。没有 padding 时输出与输入逐字节相同，报告中的 `updatedOffsets` 为 0。

## 保守拒绝

分片或分段容器含有本工具未安全覆盖的绝对/相对偏移，遇到以下结构会拒绝改写：

- 顶层 `moof`、`mfra`、`sidx`、`ssix`、`styp`
- `moov/mvex`
- `saio` 辅助数据偏移
- `iloc` 绝对 item 偏移

未知 box 按不透明载荷处理。工具保证其字节不变，但不会猜测未知格式内部的私有
偏移语义；若私有扩展自身保存了指向后续媒体数据的绝对文件偏移，应在业务侧确认
该扩展可随 padding 删除而平移，或先将 padding 固定在扩展之后。

## 真实音视频验证

`testdata/av.mp4` 是 1.2 秒 H.264 + AAC 双轨样本。测试会注入两个顶层 `free`
padding、一个私有 `Xtra` box 和一个 `uuid` 扩展，然后执行安全删除。

本次运行结果：

- 文件从 15,727 字节降为 15,647 字节，删除 3 个 `free` box。
- 重写 22 个 `stco` 表项，视频 12 个样本、音频 20 个样本。
- 视频轨样本聚合哈希未变：`b715b2b83334e54421c38ab5e411db11a57301b595db3084ba217481ff6f382a`。
- 音频轨样本聚合哈希未变：`04f73768268676cf05ab5bbf190414a52725b76c61f0df79065c23500ac16b2f`。
- `Xtra` 与 `uuid` 未知块 SHA-256 前后一致。
- ffmpeg 解码帧 `framemd5` 前后一致：
  视频 `348ff108a8b0fb9a5ea24bde8d9bf6d846b580a87ebad16ebe73accf6720386d`，
  音频 `074c5b7a896af960911be049a1919d6d16f331b49d9335c0afba6e6ae3783096`。

## 测试

```bash
go test ./...
go vet ./...
```

测试覆盖 32 位 `stco`、64 位 `co64`、双轨样本映射、未知 `uuid`/私有 box、
padding 位于 `mdat` 前后、幂等改写、分片容器拒绝，以及真实 H.264/AAC 样本。
项目运行时仅使用 Go 标准库；`testdata/av.mp4` 由 ffmpeg 生成，但测试本身不调用
ffmpeg。

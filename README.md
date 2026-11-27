# dsdtable

用于检查 DFF（DSDIFF）与 DSF 的声道交织方式，并建立逐声道样本表。
实现仅依赖 Go 标准库，不加载整段音频即可解析；提取时才按块读取有效样本。

## 快速使用

```bash
go run ./cmd/dsdtable track.dsf
go run ./cmd/dsdtable -json track.dff
go run ./cmd/dsdtable -extract ./samples track.dsf
cat track.dsf | go run ./cmd/dsdtable -
```

默认输出按声道列出有效字节、有效 1-bit 样本数、padding、缺失字节以及
每个 segment 的文件偏移、stride、块大小和尾部状态。`-json` 输出完整、
稳定的结构，适合后续转换器直接读取。`-extract` 每个声道写一个 `*.chN.raw`，
文件里只有有效打包字节，不含任何 padding。

## 核心规则

DFF：

- `FRM8` 内的 `PROP/SND` 提供 `FS  `、`CHNL`、`CMPR`。
- 原始音频位于 `DSD ` chunk，按“每帧每声道 1 字节”交织，字节内 MSB-first。
- 帧数取 `payload_size / channel_count`；不足一帧的余数字节按声道顺序归属。
- 未收到余数字节的声道写入 `missing-channel-tail`，标记为 `synthetic: true`。
- chunk 声明的数据长度为奇数时，chunk 外的补齐字节记为 `chunk-alignment`，
  不会算进任何声道。

DSF：

- `fmt ` 的 `sample count` 是每声道 1-bit 样本数；有效字节数为
  `ceil(sample_count / 8)`。
- 数据按 `block_size per channel` 分块交织：同一块组内先 ch0、再 ch1，依此类推。
- 最后一组支持三种布局：
  - `block-padded`：每个声道末块补足到 `block_size`；
  - `tail-packed`：末组只保存各声道的有效字节，不补块；
  - `block-padded-short` / `truncated-full-block`：数据提前结束，缺失部分明确标记。
- 末块中、最后一个有效字节之后的真实字节记为 `dsf-block-tail`。
- `sample_count` 不是 8 的倍数时，末字节内未使用的位记为
  `packed-byte-tail-bits`，`length` 为 0、`bits` 表示位数。

`ValidBytes` 包含承载最后若干有效位的那个打包字节；`ValidSamples` 使用精确的
1-bit 样本数，因此两者不能简单地用 `ValidBytes * 8` 互相换算。

## Padding 语义

每条 `Padding` 都有 `kind`、chunk/声道号、文件偏移、长度、位数和原因：

- `synthetic: false`：padding 真实存在于文件中，偏移可直接定位。
- `synthetic: true`：padding 不存在于文件，仅表示为了补齐尾部所需的零填充；
  提取器不会伪造这些字节。

因此“声道最后一块错位”可以按文件偏移直接核对，而不会被静默的零填充掩盖。

## Go API

```go
import (
    "fmt"
    "log"
    "os"

    "dsdtable/dsd"
)

manifest, err := dsd.ParseFile("track.dff")
if err != nil {
    log.Fatal(err)
}
fmt.Println(manifest.ChannelTables[0].ValidSamples)

file, _ := os.Open("track.dff")
defer file.Close()
samples, err := manifest.ExtractChannel(file, 0)
```

主要类型位于 `dsd/model.go`：`Manifest`、`ChannelTable`、`Segment`、`Padding`。

## 验证

```bash
go test ./...
```

测试覆盖多声道 DFF 残帧、多个 DFF 音频 chunk 的相位连续性、DSF 规范块填充、
无填充紧凑尾块、非 2 声道与非标准块长度，以及截断尾块逐声道缺失标记。

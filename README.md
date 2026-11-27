# 5.1 低频路由与反相检查

用 Go 标准库分析 5.1 WAV 中 `LFE`、`L`、`R`、`C` 的低频相关性、极性、时差和增益。目标是把两类现象分开：

- LFE 内容被错误送到主声道，或主声道低频与 LFE 存在异常耦合。
- 主声道本来就有正常的低频音乐，不能仅凭“低频很多”判成 LFE 泄漏。

程序读取 WAV 的编码声道顺序（`WAVE_FORMAT_EXTENSIBLE` 的 channel mask，或标准无 mask 5.1 顺序 `FL FR FC LFE BL BR`），对四个声道做 20-120 Hz 四阶 Butterworth 限带，再计算归一化互相关、最佳时延和最小二乘增益。

## 样本结论

对本目录的 `artifacts/fixtures/*.wav` 运行后得到：

| 样本 | 低频证据 | 结论 |
|---|---|---|
| `normal_music.wav` | 四声道低频 RMS 约 `-11` 到 `-48 dBFS`，所有审计对 `abs(r) <= 0.014` | 正常。没有相干 LFE 路由、反相或延时证据 |
| `lfe_rerouted.wav` | LFE `-45.36 dBFS`；L/R/C 与 LFE 在 `0 ms` 处 `r=+1.000`，LFE 到主声道增益约 `+32` 到 `+34 dB` | LFE 波形被重路由到 L/R/C，优先检查总线交叉点或上混/下混矩阵 |
| `inverted.wav` | L/R 为 `+1.000`；C/LFE 与 L/R 在 `0 ms` 处为 `-1.000`，C 与 LFE 彼此为 `+1.000` | C、LFE 与 L/R 参考组反相；检查这两路极性 |
| `delay_mismatch.wav` | L/LFE 在 `+22.000 ms`（1056 samples）处 `r=+1.000`，LFE 比 L 低 `6.02 dB` | LFE 相对 L 有约 `22 ms` 延时；检查 LFE 链路的延迟补偿 |

`delay_mismatch.wav` 的中心声道低频只有 `-59.81 dBFS`，主要是高频内容，不参与低频故障判断。纯音或窄带低频在互相关上可能出现“整周期”多解，程序会给该样本附加时延模糊提示。

`inverted.wav` 没有独立绝对极性参考，因此结论采用 L/R 作为参考组：C/LFE 这一组与 L/R 相反。若实际参考是 C/LFE，则应反过来检查 L/R。

## 判定规则

- 路由/泄漏：LFE 很低（`<= -35 dBFS`，或比主声道低 `20 dB` 以上），且 LFE 与某个主声道 `abs(r) >= 0.85`、LFE 到主声道的增益 `>= +12 dB`、时差绝对值 `<= 1 ms`。
- 反相：两个活动声道 `abs(r) >= 0.90`，最佳时差 `<= 0.10 ms`，但相关系数为负。
- 延时：两个活动声道 `abs(r) >= 0.85`，最佳时差 `>= 0.10 ms`，且峰值不在搜索边界。
- 正常低频音乐：只检查主声道之间的共享低频时，不报泄漏；只有在与 LFE 形成强相干并伴随异常增益/电平时才给出 LFE 路由证据。若 LFE 已低到没有可用参考，程序明确提示“仅凭信号无法证明重路由”。

这些阈值是针对节目总线审计的保守起点，不是响度或母版规范。需要改频段时使用命令行参数。

## 构建与运行

```bash
go run ./cmd/lfecheck artifacts/fixtures/*.wav
go run ./cmd/lfecheck --json artifacts/fixtures/lfe_rerouted.wav
go run ./cmd/lfecheck --markdown artifacts/fixtures/*.wav > out/report.md
```

常用参数：

```text
--json                 输出 JSON；单文件为一个对象，多文件为数组
--markdown             输出 Markdown 表格
--layout L,R,C,LFE,... 覆盖 WAV 中推断出的编码顺序
--low-hz 20            低频带高通边界
--high-hz 120          低频带低通边界
--max-lag-ms 50        互相关最大搜索时差
```

退出码：

- `0`：没有问题 finding。
- `1`：至少一个问题 finding。
- `2`：参数、WAV 读取或格式错误。

支持 PCM 8/16/24/32-bit、IEEE float 32/64-bit 和 `WAVE_FORMAT_EXTENSIBLE`。无第三方依赖。

## Go 库

```go
result, err := lfcheck.AnalyzeFile("program.wav", lfcheck.Options{})
if err != nil {
    // handle input error
}
for _, finding := range result.Findings {
    fmt.Println(finding.Code, finding.Channels, finding.Message)
}
```

`lfcheck.Options` 可设置 `LowHz`、`HighHz`、`MaxLagMS` 和 `Layout`。结果中保留全部六对信道的 `ZeroLag`、`Best`、`LagMS`、`LagSamples`、`GainDB`、`ReverseGainDB` 和 `LagReliable`，便于人工复核阈值边缘案例。

## 测试

```bash
go test ./...
go vet ./...
```

测试包含正常独立低频、主声道共享音乐、低 LFE 且无参考、LFE 重路由、反相、5 ms 延时，以及 16-bit PCM 和 extensible WAV 解析。

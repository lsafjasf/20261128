# PTP 主备切换样本连续性仿真

纯 Go 标准库实现的确定性仿真，用 announce、sync、delay request/response
复现 PTP master 到 backup 的切换，并判断两台录音设备的样本计数差来自
时间戳、重采样还是缓存重排。

## 运行

```bash
go run ./cmd/ptpsim
go run ./cmd/ptpsim -events
go run ./cmd/ptpsim -json
go test ./...
```

程序只依赖 Go 标准库（`flag`、`encoding/json`、`sort`、`math` 等），不需要
下载第三方模块。

## 默认场景

- 采样率 96 kHz，master 在 1.5 s 停止，backup 在 2.0 s 发出 announce，
  3.0 s 重新锁定，失锁窗口 1.5 s。
- 录音端 A/B 相对 PTP 的本地时钟偏差分别为 +120 ppm 和 -150 ppm。
- 失锁期间 ASRC 比例冻结，恢复后只恢复校正，不补写历史缺口。
- 同时注入非对称 delay 路径和到达乱序，作为时间戳与缓存重排的对照。

默认结果：A 写入 480017 个样本，B 写入 479978 个样本，差 39 个样本。
时间戳只产生 4.8 个样本的标签偏斜；260 次乱序重排后净样本差为 0。
因此判定为重采样失锁，而不是时间戳或缓存重排。

恢复锁定不等于允许直接拼接。原始流存在 39 个样本的不连续，必须先按
公共时基重采样并按 PTP 时间戳裁剪；详见 `REPORT.md`。

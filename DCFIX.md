# dcfix — DC 偏置 / 渐漂 / 超低频分析与修复工具

现场录音的直流偏置在后续增益级之后会导致单侧削波，而普通峰值表只显示
`max(|peak|)`，掩盖了正负两侧余量的不对称。`dcfix` 逐声道估计 DC offset、
低频趋势与**双侧**有效余量，区分三类成分，并在修复后逐段记录源样本差异。

## 用法

```sh
go build -o dcfix .
# 分析：每声道输出 dc_offset、drift、subsonic、双侧余量、分类
./dcfix analyze -json report.json in.wav
# 修复：默认 drift 模式 + 保留真实超低频，输出逐段差异报告
./dcfix fix -json fix.json in.wav out.wav
# 生成合成演示文件（ch0: +0.08 固定偏置 + 0.12 渐漂；ch1: -0.06 固定 + 15Hz 超低频）
./dcfix demo demo.wav
```

## 分析口径

- **分段**：0.5s 一段，段均值为该段 DC 估计。
- **固定偏置（fixed）**：全文件段均值的中位数，阈值 -60 dBFS。
- **渐漂（drift）**：段均值对时间做线性回归，`drift_total` 为全程漂移量；
  要求回归残差显著小于漂移量（否则只是噪声起伏）。
- **真实超低频（subsonic）**：零相位 20Hz 低通提取超低频带，再去掉其
  DC+线性趋势，剩余 RMS 即为真实超低频能量。先低通后去趋势的顺序保证
  渐漂不会被误判为超低频，节目内容（>=20Hz）也不会泄漏进来。
- **有效余量**：`headroom_pos_db` / `headroom_neg_db` 分别为到 +FS / -FS
  的余量，`effective_headroom_db` 取较差一侧——即峰值表看不到、但增益后
  先削波的那一侧。

## 修复与差异记录

- `fixed` 模式减去常数 DC；`drift` 模式减去段均值线性插值的慢变趋势
  （同时覆盖固定偏置与渐漂）；`subsonic` 模式保留接口用于同时去除超低频。
- `-keep-subsonic`（默认开）：校正曲线再经 20Hz 低通，只移除超慢成分，
  真实 <20Hz 节目内容（如风声、机械振动）不被误删。
- 逐段差异记录于修复报告的 `repairs[].segments[]`：每段给出
  `changed_samples`、`max_abs_diff`、`rms_diff`、`mean_diff`，
  即源样本与修复样本的逐段差异；`post_fix_analysis` 给出修复后复检结果。
- 输出默认 32-bit float，避免修复后重新量化。

## 测试

```sh
go test ./...
```

覆盖：纯渐漂不漏入超低频、220Hz 节目不漏入超低频、15Hz 真实超低频
估计误差 <30%、修复后残余 DC <1e-3 且逐段差异记录完整。

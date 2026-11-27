# frameflow — 四机位掉帧根因定位

纯 Go 实现，不依赖任何外部程序。把四路的相机时间戳、采集队列深度、
编码耗时、存储写入耗时合并到同一时间线，按时间先后找出首个偏离基线的子系统。

## 用法

```sh
go build -o frameflow .
./frameflow simulate -o logs.jsonl   # 生成仿真日志（ch2 在 t=4s 注入存储故障）
./frameflow analyze logs.jsonl       # 合并时间线 + 定位首个偏离子系统
go test ./...                        # 端到端验证
```

## 日志格式（JSONL，可替换为真实采集日志）

每行一条记录，四路共用同一时间基准（`ts` 为秒）：

```json
{"ts":4.0,"ch":2,"sub":"camera","frame":120}
{"ts":4.0,"ch":2,"sub":"capture","frame":120,"qdepth":3,"drop":false}
{"ts":4.0,"ch":2,"sub":"encode","frame":120,"dur_ms":4.2}
{"ts":4.0,"ch":2,"sub":"storage","frame":120,"write_ms":70.4}
{"ts":10.0,"ch":2,"sub":"file","frame":-1,"frames":187}
```

## 判定逻辑

- 以首个 1 秒为健康基线，按 200ms 分桶对齐四路指标。
- 偏离阈值：相机出帧 < 基线 70%；队列深度 > 容量 75% 或出现丢帧；
  编码耗时 > 2× 基线；写入耗时 > 3× 基线。
- 所有偏离按时间排序，最早者即根因；其余为反压向上游传播的结果。

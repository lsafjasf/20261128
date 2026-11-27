package ptpsim

import (
	"fmt"
	"time"
)

func Run(cfg Config) Report {
	report := simulate(cfg)

	tsCfg := cfg
	tsCfg.Name = cfg.Name + "-timestamp-only"
	tsCfg.ClockPPM.A, tsCfg.ClockPPM.B = 0, 0
	tsCfg.Reorder.Enabled = false
	timestampOnly := simulate(tsCfg)

	rsCfg := cfg
	rsCfg.Name = cfg.Name + "-resampling-only"
	rsCfg.PathAsymmetry.A, rsCfg.PathAsymmetry.B = 0, 0
	rsCfg.Reorder.Enabled = false
	resamplingOnly := simulate(rsCfg)

	roCfg := cfg
	roCfg.Name = cfg.Name + "-reorder-only"
	roCfg.ClockPPM.A, roCfg.ClockPPM.B = 0, 0
	roCfg.PathAsymmetry.A, roCfg.PathAsymmetry.B = 0, 0
	reorderOnly := simulate(roCfg)

	report.Counterfactuals = []Counterfactual{
		counterfactual("timestamp-only", timestampOnly),
		counterfactual("resampling-only", resamplingOnly),
		counterfactual("reorder-only", reorderOnly),
	}
	report.Diagnosis = diagnose(cfg, report, report.Counterfactuals)
	return report
}

func counterfactual(name string, report Report) Counterfactual {
	return Counterfactual{
		Name:       name,
		Counts:     report.Counts,
		Timestamp:  report.Timestamp,
		Resampling: report.Resampling,
		Buffer:     report.Buffer,
	}
}

func diagnose(cfg Config, report Report, controls []Counterfactual) Diagnosis {
	ts := controls[0]
	rs := controls[1]
	ro := controls[2]
	observed := report.Counts.AbsDelta

	contributions := []string{
		fmt.Sprintf("时间戳对照：样本差 %d，仅造成 %.2f 样本的标签偏斜；时间戳不生成/删除样本。",
			ts.Counts.AbsDelta, report.Timestamp.SkewSamples),
		fmt.Sprintf("重采样对照：样本差 %d，失锁窗口 %.3fs 内 ASRC 比例冻结，本地时钟相对漂移 %.1f ppm。",
			rs.Counts.AbsDelta, seconds(report.LossWindow),
			cfg.ClockPPM.A-cfg.ClockPPM.B),
		fmt.Sprintf("缓存重排对照：样本差 %d，%d 次到达逆序经序号重排后完全复原。",
			ro.Counts.AbsDelta, report.Buffer.ArrivalInversions),
	}

	d := Diagnosis{
		Verdict:        "无法由样本差唯一判因",
		DominantCause:  "unknown",
		Confidence:     "low",
		Contributions:  contributions,
		SeamlessSplice: observed == 0,
	}
	switch {
	case observed == 0:
		d.Verdict = "没有可判定的样本计数差"
	case rs.Counts.AbsDelta == observed && ts.Counts.AbsDelta == 0 && ro.Counts.AbsDelta == 0:
		d.Verdict = "重采样失锁（ASRC 冻结），不是时间戳或缓存重排"
		d.DominantCause = "resampling"
		d.Confidence = "high"
		d.SeamlessSplice = false
	case ro.Counts.AbsDelta == observed && ts.Counts.AbsDelta == 0 && rs.Counts.AbsDelta == 0:
		d.Verdict = "缓存重排造成净样本变化（存在丢包或重复包）"
		d.DominantCause = "buffer_reorder"
		d.Confidence = "high"
		d.SeamlessSplice = false
	case ts.Counts.AbsDelta == observed && rs.Counts.AbsDelta == 0 && ro.Counts.AbsDelta == 0:
		d.Verdict = "时间戳路径存在计数裁剪，标签/时间戳不可直接使用"
		d.DominantCause = "timestamp"
		d.Confidence = "high"
		d.SeamlessSplice = false
	default:
		d.Verdict = "多种误差叠加；以重采样为主要贡献"
		d.DominantCause = "mixed"
		d.Confidence = "medium"
		d.SeamlessSplice = false
	}

	if d.SeamlessSplice {
		d.SpliceGuidance = "原始样本在计数上对齐，但仍须核对边界时间戳与内容连续性后才能拼接。"
	} else {
		d.SpliceGuidance = fmt.Sprintf(
			"不允许直接无缝拼接：先对失锁窗口做 ASRC/公共时基重采样并按 PTP 时间戳裁剪，补偿 %d 样本的计数不连续；仅修正时间戳或重排缓冲不能消除该跳变。",
			observed)
	}
	return d
}

func seconds(d time.Duration) float64 {
	return float64(d) / 1e9
}

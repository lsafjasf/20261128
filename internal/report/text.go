package report

import (
	"fmt"
	"io"
	"math"
	"strings"

	"resampleforensics/internal/analysis"
)

// Text writes a human-readable report. It intentionally presents latency and
// edge metrics before any attribution.
func Text(w io.Writer, r analysis.Report) error {
	if _, err := fmt.Fprintf(w, "重采样时域取证\n"); err != nil {
		return err
	}
	fmt.Fprintf(w, "源文件: %d Hz, %d 样本, %.3f s, peak %.4f, RMS %.4f\n",
		r.Source.SampleRate, r.Source.Samples, r.Source.DurationMS/1000, r.Source.Peak, r.Source.RMS)
	fmt.Fprintf(w, "目标:   %d Hz, %d 样本, %.3f s, peak %.4f, RMS %.4f\n",
		r.Destination.SampleRate, r.Destination.Samples, r.Destination.DurationMS/1000, r.Destination.Peak, r.Destination.RMS)
	fmt.Fprintf(w, "转换:   %s\n", r.Conversion)
	fmt.Fprintf(w, "对齐:   目标相对源滞后 %+.3f 样本 (%+.4f ms), 置信度 %.3f\n",
		r.Alignment.LagSamples, r.Alignment.LagMS, r.Alignment.Confidence)
	fmt.Fprintf(w, "方法:   %s\n", r.Alignment.Method)
	fmt.Fprintf(w, "约束:   所有相位/误差指标先按延迟对齐；未使用整段平均频谱相似度作为判据。\n\n")

	fmt.Fprintf(w, "群延迟（延迟引用；Relative 是整数对齐后的残余）\n")
	fmt.Fprintf(w, "%-10s %11s %10s %10s %10s %8s\n", "频段", "范围Hz", "相对ms", "总ms", "波动ms", "相位RMS°")
	for _, band := range r.GroupDelay.Bands {
		rangeText := fmt.Sprintf("%.0f-%.0f", band.LowHz, band.HighHz)
		fmt.Fprintf(w, "%-10s %11s %10.4f %10.4f %10s %8.2f\n",
			band.Name, rangeText, band.GroupDelayMS, band.TotalDelayMS, num(band.RippleMS, "%.4f"), band.PhaseRMSError)
	}
	fmt.Fprintf(w, "全带相位线性 RMS: %.3f°, p95-p5 群延迟波动: %s ms, 有效 3 dB 带宽: %.0f Hz\n",
		r.GroupDelay.PhaseLinearityRMSDeg, num(r.GroupDelay.RippleMS, "%.4f"), r.GroupDelay.EffectiveBandwidthHz)
	fmt.Fprintf(w, "相干门限 %.2f, 有效频点 %d, 相位曲线可信: %v\n\n",
		r.GroupDelay.CoherenceGate, r.GroupDelay.ValidPassbandBins, r.GroupDelay.PhaseCurveIsReliable)

	fmt.Fprintf(w, "相对脉冲响应\n")
	fmt.Fprintf(w, "  -40 dB 支撑: %.4f ms; -60 dB 支撑: %.4f ms (%.1f 目标样本); 99%% 能量: %.4f ms\n",
		r.Impulse.Support40DBMS, r.Impulse.Support60DBMS, r.Impulse.Support60Samples, r.Impulse.Energy99MS)
	fmt.Fprintf(w, "  前/后能量: %s dB; 对称性: %s; 峰残余: %+.4f ms; 线性相位: %v\n",
		num(r.Impulse.PreEnergyDB, "%+.2f"), num(r.Impulse.Symmetry, "%.3f"), r.Impulse.PeakResidualMS, r.Impulse.LinearPhaseLikely)
	if !math.IsNaN(r.Impulse.LatencyImpliedTaps) {
		fmt.Fprintf(w, "  由未补偿延迟推断的线性相位长度: %.1f taps\n", r.Impulse.LatencyImpliedTaps)
	}
	fmt.Fprintf(w, "  注: %s\n\n", r.Impulse.TapsNote)

	fmt.Fprintf(w, "瞬态（宽度为 10%% 包络，Rise 为 10%%→90%%）\n")
	if len(r.Transients.Events) == 0 {
		fmt.Fprintf(w, "  未找到可稳定测量的瞬态。\n\n")
	} else {
		fmt.Fprintf(w, "%-3s %8s %9s %9s %9s %8s %9s %10s %10s\n",
			"#", "时间s", "源宽ms", "参考宽ms", "目标宽ms", "宽比", "上升比", "首移ms", "尾移ms")
		for i, e := range r.Transients.Events {
			fmt.Fprintf(w, "%-3d %8.3f %9.3f %9.3f %9.3f %8s %9s %+10.4f %+10.4f\n",
				i+1, e.TimeSec, e.SourceWidthMS, e.RefWidthMS, e.DstWidthMS,
				num(e.WidthRatio, "%.3f"), num(e.RiseRatio, "%.3f"), e.LeadShiftMS, e.TailShiftMS)
			fmt.Fprintf(w, "    首/尾相位误差: %+.1f° / %+.1f°; 前/后能量 源 %s dB, 参考 %s dB, 目标 %s dB\n",
				e.LeadPhaseDeg, e.TailPhaseDeg, num(e.SourcePrePostDB, "%.2f"),
				num(e.RefPrePostDB, "%.2f"), num(e.DstPrePostDB, "%.2f"))
		}
		fmt.Fprintf(w, "  汇总: 宽比中位 %s / P90 %s, 上升比中位 %s, 首移 %.4f ms, 尾移 %.4f ms\n",
			num(r.Transients.MedianWidthRatio, "%.3f"), num(r.Transients.P90WidthRatio, "%.3f"),
			num(r.Transients.MedianRiseRatio, "%.3f"), r.Transients.MedianLeadShiftMS, r.Transients.MedianTailShiftMS)
		fmt.Fprintf(w, "  源内容已宽: %v, 重采样链路增宽: %v, 检测数: %d\n\n",
			r.Transients.SourceAlreadyBroad, r.Transients.ConverterWidening, r.Transients.DetectedCount)
	}

	fmt.Fprintf(w, "边界处理（匹配候选半宽 + zero/reflect/clamp）\n")
	fmt.Fprintf(w, "  最佳: %s, 半宽 %d 源样本, 模型间余量 %.2f%%, 判定不确定: %v\n",
		r.Boundary.BestMode, r.Boundary.BestHalfWidth, 100*r.Boundary.BestMargin, r.Boundary.Indeterminate)
	fmt.Fprintf(w, "  边界窗口 %.1f ms; 内部 NRMSE %.5f; 前/尾额外 %.3f/%.3f ms\n",
		r.Boundary.WindowMS, r.Boundary.InteriorNRMSE, r.Boundary.LeadingExtraMS, r.Boundary.TrailingExtraMS)
	limit := len(r.Boundary.Scores)
	if limit > 8 {
		limit = 8
	}
	for _, s := range r.Boundary.Scores[:limit] {
		fmt.Fprintf(w, "  %-7s 半宽 %3d: 首 %s, 尾 %s, 合计 %s\n",
			s.Mode, s.HalfWidth, num(s.Start, "%.5f"), num(s.End, "%.5f"), num(s.Combined, "%.5f"))
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "归因: %s (置信度 %.2f)\n", classificationZH(r.Attribution.Classification), r.Attribution.Confidence)
	for _, e := range r.Attribution.Evidence {
		fmt.Fprintf(w, "  - %s\n", e)
	}
	if len(r.Attribution.Limitations) > 0 {
		fmt.Fprintf(w, "限制:\n")
		for _, lim := range r.Attribution.Limitations {
			fmt.Fprintf(w, "  - %s\n", lim)
		}
	}
	if len(r.Warnings) > 0 {
		fmt.Fprintf(w, "警告:\n")
		for _, warning := range r.Warnings {
			fmt.Fprintf(w, "  - %s\n", warning)
		}
	}
	return nil
}

func num(v float64, format string) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "n/a"
	}
	return fmt.Sprintf(format, v)
}

func classificationZH(v string) string {
	switch strings.ToLower(v) {
	case "resampler":
		return "重采样链路导致（滤波/相位/边界）"
	case "source_content":
		return "源文件内容已有特征"
	case "inconclusive":
		return "证据不足，无法唯一归因"
	default:
		return v
	}
}

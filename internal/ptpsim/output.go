package ptpsim

import (
	"encoding/json"
	"fmt"
	"io"
)

func WriteJSON(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func WriteReport(w io.Writer, report Report) {
	cfg := report.Config
	fmt.Fprintln(w, "PTP 主备切换仿真")
	fmt.Fprintf(w, "场景: %s\n", cfg.Name)
	fmt.Fprintf(w, "采样率: %.0f Hz | 本地时钟偏差: A=%+.1f ppm B=%+.1f ppm\n",
		cfg.SampleRate, cfg.ClockPPM.A, cfg.ClockPPM.B)
	fmt.Fprintf(w, "master 停止: %.3fs | backup announce: %.3fs | 重新锁定: %.3fs\n",
		seconds(cfg.MasterStop), seconds(cfg.BackupAnnounce), seconds(cfg.RelockAt))
	fmt.Fprintf(w, "失锁窗口: %.3fs\n\n", seconds(report.LossWindow))

	fmt.Fprintln(w, "时间线")
	for _, segment := range report.Timeline {
		fmt.Fprintf(w, "  %6.3f-%6.3fs  %-20s %s\n",
			seconds(segment.Start), seconds(segment.End), segment.State, segment.Detail)
	}

	fmt.Fprintln(w, "\nPTP 消息")
	fmt.Fprintf(w, "  announce=%d sync=%d delay_request=%d delay_response=%d\n",
		report.Messages[Announce], report.Messages[Sync],
		report.Messages[DelayReq], report.Messages[DelayResp])

	fmt.Fprintln(w, "\n录音端计数")
	for _, rec := range report.Recorders {
		fmt.Fprintf(w, "  %s: written=%d samples | holdover=%.2f samples | ASRC frozen ticks=%d\n",
			rec.ID, rec.WrittenSamples, rec.HoldoverSamples, rec.ResamplerFrozenSteps)
	}
	fmt.Fprintf(w, "  B-A=%d samples | |B-A|=%d samples\n", report.Counts.Delta, report.Counts.AbsDelta)

	fmt.Fprintln(w, "\n三假设证据")
	fmt.Fprintf(w, "  时间戳: offset skew=%s (%.2f samples), count effect=%d\n",
		report.Timestamp.Skew, report.Timestamp.SkewSamples, report.Timestamp.CountEffect)
	fmt.Fprintf(w, "  重采样: enabled=%t frozen_during_loss=%t count effect=%d\n",
		report.Resampling.Enabled, report.Resampling.FrozenDuringLoss, report.Resampling.CountEffect)
	fmt.Fprintf(w, "  缓存重排: inversions=%d drops=%d duplicates=%d net sample delta=%d\n",
		report.Buffer.ArrivalInversions, report.Buffer.Drops,
		report.Buffer.Duplicates, report.Buffer.NetSampleDelta)

	fmt.Fprintln(w, "\n对照回放")
	for _, control := range report.Counterfactuals {
		fmt.Fprintf(w, "  %-16s sample_delta=%d timestamp_skew=%.2f reorder_inversions=%d\n",
			control.Name, control.Counts.AbsDelta,
			control.Timestamp.SkewSamples, control.Buffer.ArrivalInversions)
	}

	fmt.Fprintln(w, "\n结论")
	for _, item := range report.Diagnosis.Contributions {
		fmt.Fprintf(w, "  - %s\n", item)
	}
	fmt.Fprintf(w, "判定: %s (confidence=%s, dominant=%s)\n",
		report.Diagnosis.Verdict, report.Diagnosis.Confidence, report.Diagnosis.DominantCause)
	fmt.Fprintf(w, "恢复后直接无缝拼接: %t\n", report.Diagnosis.SeamlessSplice)
	fmt.Fprintf(w, "拼接要求: %s\n", report.Diagnosis.SpliceGuidance)
}

func PrintEvents(w io.Writer, events []Message) {
	fmt.Fprintln(w, "PTP event log")
	for _, event := range events {
		fmt.Fprintf(w, "%8.6fs %-10s %-15s -> %-15s seq=%-4d state=%s",
			seconds(event.At), event.Type, event.Source, event.Target,
			event.Sequence, event.ClockState)
		if event.Grandmaster != "" {
			fmt.Fprintf(w, " gm=%s priority1=%d", event.Grandmaster, event.Priority1)
		}
		if event.Correction != 0 {
			fmt.Fprintf(w, " correction=%s", event.Correction)
		}
		fmt.Fprintln(w)
	}
}

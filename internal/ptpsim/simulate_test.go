package ptpsim

import (
	"testing"
	"time"
)

func TestDefaultFailoverDiagnosis(t *testing.T) {
	report := Run(DefaultConfig())

	if report.LossWindow != 1500*time.Millisecond {
		t.Fatalf("loss window = %s, want 1.5s", report.LossWindow)
	}
	if report.Counts.AbsDelta != 39 {
		t.Fatalf("sample delta = %d, want 39", report.Counts.AbsDelta)
	}
	if report.Counts.A != 480017 || report.Counts.B != 479978 {
		t.Fatalf("sample counts A=%d B=%d, want A=480017 B=479978",
			report.Counts.A, report.Counts.B)
	}
	if report.Timestamp.CountEffect != 0 {
		t.Fatalf("timestamp count effect = %d, want 0", report.Timestamp.CountEffect)
	}
	if !report.Resampling.FrozenDuringLoss {
		t.Fatal("resampler should be frozen during loss of lock")
	}
	if report.Buffer.NetSampleDelta != 0 {
		t.Fatalf("reorder net sample delta = %d, want 0", report.Buffer.NetSampleDelta)
	}
	if report.Buffer.ArrivalInversions == 0 {
		t.Fatal("scenario should contain observable packet reordering")
	}
	if report.Diagnosis.DominantCause != "resampling" {
		t.Fatalf("dominant cause = %q, want resampling", report.Diagnosis.DominantCause)
	}
	if report.Diagnosis.SeamlessSplice {
		t.Fatal("raw seamless splice must not be allowed with a 39-sample discontinuity")
	}
}

func TestRequiredPTPMessagesAndRecovery(t *testing.T) {
	report := Run(DefaultConfig())
	for _, messageType := range []MessageType{Announce, Sync, DelayReq, DelayResp} {
		if report.Messages[messageType] == 0 {
			t.Fatalf("missing %s messages", messageType)
		}
	}
	if report.Messages[Sync] != 45 {
		t.Fatalf("sync count = %d, want 45", report.Messages[Sync])
	}

	last := report.Timeline[len(report.Timeline)-1]
	if last.State != "locked_backup" || last.End != 5*time.Second {
		t.Fatalf("last phase = %#v, want locked_backup through 5s", last)
	}
	for _, event := range report.Events {
		if event.At >= 3*time.Second && event.Type == Sync {
			if event.Source != backupMaster || event.ClockState != "locked_backup" {
				t.Fatalf("post-recovery sync = %#v, want backup locked", event)
			}
		}
	}
}

func TestCounterfactualsIsolateCauses(t *testing.T) {
	report := Run(DefaultConfig())
	if len(report.Counterfactuals) != 3 {
		t.Fatalf("counterfactual count = %d, want 3", len(report.Counterfactuals))
	}

	ts := report.Counterfactuals[0]
	rs := report.Counterfactuals[1]
	ro := report.Counterfactuals[2]
	if ts.Counts.AbsDelta != 0 || ts.Timestamp.SkewSamples < 1 {
		t.Fatalf("timestamp-only control = %#v, want label skew with zero count delta", ts)
	}
	if rs.Counts.AbsDelta != report.Counts.AbsDelta || rs.Timestamp.SkewSamples != 0 {
		t.Fatalf("resampling-only control = %#v, want count delta without timestamp skew", rs)
	}
	if ro.Counts.AbsDelta != 0 || ro.Buffer.ArrivalInversions == 0 {
		t.Fatalf("reorder-only control = %#v, want inversions with zero count delta", ro)
	}
}

func TestReorderReassemblyKeepsSampleCount(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Reorder.Drops = 0
	cfg.Reorder.Duplicates = 0
	metrics := simulateBuffer(cfg)
	if metrics.ArrivalInversions == 0 {
		t.Fatal("expected reordered arrivals")
	}
	if metrics.SamplesBefore != metrics.SamplesAfter || metrics.NetSampleDelta != 0 {
		t.Fatalf("reorder changed sample count: before=%d after=%d delta=%d",
			metrics.SamplesBefore, metrics.SamplesAfter, metrics.NetSampleDelta)
	}
}

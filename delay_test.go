package captionsession

import "testing"

func TestLatencyEstimatorConverges(t *testing.T) {
	// Speech ending at PTS t arrives at wall time t+2000ms: steady 2s lag.
	est := NewLatencyEstimator()
	base := int64(10_000)
	for i := 0; i < 30; i++ {
		endPTS := base + int64(i)*1000
		est.Observe(endPTS+2000, endPTS)
	}
	got := est.EstimateMS(2000)
	if got < 1900 || got > 2100 {
		t.Fatalf("estimated delay %dms, want ~2000ms", got)
	}
}

func TestLatencyEstimatorAssumedBeforeSamples(t *testing.T) {
	est := NewLatencyEstimator()
	if got := est.EstimateMS(1500); got != 1500 {
		t.Fatalf("assumed delay = %d, want 1500", got)
	}
	// Non-positive delay is ignored.
	est.Observe(100, 200)
	if est.Samples() != 0 {
		t.Fatalf("negative sample must be ignored")
	}
}

func TestLatencyEstimatorDriftClamp(t *testing.T) {
	est := NewLatencyEstimatorWith(0.5, 1000)
	est.Observe(3000, 1000)      // 2000ms delay
	est.Observe(9_000_000, 2000) // absurd 8,998,000ms jump
	got := est.EstimateMS(2000)
	if got > 4000 {
		t.Fatalf("drift clamp failed, estimate = %d", got)
	}
}

func TestPlayheadCompensatesLag(t *testing.T) {
	// At wall time 12000 with 2000ms pipeline delay, the spoken PTS is 10000.
	if got := Playhead(12000, 2000); got != 10000 {
		t.Fatalf("playhead = %d, want 10000", got)
	}
	if got := Playhead(500, 2000); got != 0 {
		t.Fatalf("playhead must clamp at zero, got %d", got)
	}
}

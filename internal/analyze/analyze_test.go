package analyze

import "testing"

func TestDetectOnsetsFindsImpulsePairs(t *testing.T) {
	sampleRate := 1000
	signal := make([]float64, sampleRate)
	signal[200] = 40
	signal[700] = -35

	onsets := DetectOnsets(signal, sampleRate)
	if len(onsets) != 2 {
		t.Fatalf("got %d onsets, want 2: %v", len(onsets), onsets)
	}
	if abs(onsets[0]-200) > 10 || abs(onsets[1]-700) > 10 {
		t.Fatalf("onset positions outside tolerance: %v", onsets)
	}
}

func TestCompareReportsLSBScaleResidual(t *testing.T) {
	source := make([]int32, 1000)
	output := make([]int32, 1000)
	for i := range source {
		source[i] = 0
		output[i] = 0
	}
	output[500] = 1
	result, rows := Compare("test", 1000, source, output)
	if len(result.Onsets) != 0 {
		t.Fatalf("unexpected onsets: %v", result.Onsets)
	}
	if result.ResidualRMS <= 0 || result.ResidualPeak <= 0 {
		t.Fatalf("non-positive residual: %+v", result)
	}
	if len(rows) == 0 {
		t.Fatal("no frame rows emitted")
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

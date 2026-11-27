package wavio

import (
	"path/filepath"
	"testing"
)

func TestMono24RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mono24.wav")
	input := PCM{
		SampleRate: 48000,
		Samples:    []float64{-1, -0.5, 0, 0.25, 1},
	}
	if err := WriteMono24(path, input); err != nil {
		t.Fatal(err)
	}
	output, err := ReadMono24(path)
	if err != nil {
		t.Fatal(err)
	}
	if output.SampleRate != input.SampleRate || len(output.Samples) != len(input.Samples) {
		t.Fatalf("unexpected PCM header: %+v", output)
	}
	for i, sample := range input.Samples {
		difference := sample - output.Samples[i]
		if difference < 0 {
			difference = -difference
		}
		if difference > 1.01/float64(MaxInt24) {
			t.Fatalf("sample %d: got %.10f, want %.10f", i, output.Samples[i], sample)
		}
	}
}

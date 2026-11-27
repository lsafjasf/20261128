package lc3

import "testing"

func TestSamplesTable(t *testing.T) {
	cases := []struct {
		rate uint32
		dur  Duration
		want int
	}{
		{8000, Dur10ms, 80}, {8000, Dur7p5ms, 60},
		{16000, Dur10ms, 160}, {16000, Dur7p5ms, 120},
		{24000, Dur10ms, 240}, {24000, Dur7p5ms, 180},
		{32000, Dur10ms, 320}, {32000, Dur7p5ms, 240},
		{48000, Dur10ms, 480}, {48000, Dur7p5ms, 360},
	}
	for _, c := range cases {
		got, err := Samples(c.rate, c.dur)
		if err != nil {
			t.Fatalf("Samples(%d,%v) error: %v", c.rate, c.dur, err)
		}
		if got != c.want {
			t.Errorf("Samples(%d,%v) = %d, want %d", c.rate, c.dur, got, c.want)
		}
	}
}

func TestSamplesErrors(t *testing.T) {
	if _, err := Samples(44100, Dur10ms); err == nil {
		t.Error("expected error for unsupported rate 44100")
	}
	if _, err := Samples(48000, Duration(9)); err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestDurationExactness(t *testing.T) {
	// 7.5 ms is exact in microseconds but not milliseconds, which is why the
	// timeline must be computed in samples rather than integer milliseconds.
	if Dur7p5ms.Microseconds() != 7500 {
		t.Fatalf("7.5ms = %d us, want 7500", Dur7p5ms.Microseconds())
	}
	if Dur10ms.Microseconds() != 10000 {
		t.Fatalf("10ms = %d us, want 10000", Dur10ms.Microseconds())
	}
	if Dur10ms.Milliseconds() != 10 || Dur7p5ms.Milliseconds() != 7.5 {
		t.Fatal("fractional millisecond duration mismatch")
	}
}

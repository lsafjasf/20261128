package subburn

import (
	"testing"
	"time"
)

func mustRate(t *testing.T, value string) Rate {
	t.Helper()
	rate, err := ParseRate(value)
	if err != nil {
		t.Fatalf("ParseRate(%q): %v", value, err)
	}
	return rate
}

func seconds(value float64) time.Duration {
	return time.Duration(value * float64(time.Second))
}

func testCues(starts ...time.Duration) []Cue {
	cues := make([]Cue, len(starts))
	for i, start := range starts {
		cues[i] = Cue{
			ID:    "cue-" + time.Duration(i).String(),
			Start: start,
			End:   start + time.Second,
			Text:  "line",
		}
	}
	return cues
}

func TestRateUsesExactRationalFramePTS(t *testing.T) {
	rate := mustRate(t, "30000/1001")
	if got := rate.FramePTS(30000); got != 1001*time.Second {
		t.Fatalf("frame 30000 PTS = %s, want 1001s", got)
	}
	for _, frame := range []int64{0, 1, 12345, 1000000} {
		if got := rate.FrameAt(rate.FramePTS(frame)); got != frame {
			t.Fatalf("FrameAt(FramePTS(%d)) = %d", frame, got)
		}
	}
	if got := rate.FrameCount(1001 * time.Second); got != 30000 {
		t.Fatalf("frame count = %d, want 30000", got)
	}
}

func TestAccumulatedFrameStepDriftsButExactPTSDoesNot(t *testing.T) {
	rate := mustRate(t, "30000/1001")
	const frame = int64(1000000)
	exact := rate.FramePTS(frame)
	accumulated := time.Duration(frame) * rate.FrameDuration()
	if exact-accumulated < 500*time.Microsecond {
		t.Fatalf("test setup: exact=%s accumulated=%s", exact, accumulated)
	}
	if drift := rate.FramePTS(frame - 1); exact-drift > 34*time.Millisecond {
		t.Fatalf("exact frame PTS jumped by %s", exact-drift)
	}
}

func TestRateParsingKeepsNTSCRationalExact(t *testing.T) {
	ntsc := mustRate(t, "30000/1001")
	decimal := mustRate(t, "29.97")
	if ntsc.Equal(decimal) {
		t.Fatal("29.97 must not be silently treated as 30000/1001")
	}
	if got, want := decimal.String(), "2997/100"; got != want {
		t.Fatalf("29.97 = %s, want %s", got, want)
	}
}

func TestSubtitleStartUsesFirstFrameAtOrAfterCue(t *testing.T) {
	rate := mustRate(t, "25")
	if got := rate.FrameAtOrAfter(time.Second); got != 25 {
		t.Fatalf("exact boundary frame = %d, want 25", got)
	}
	if got := rate.FrameAtOrAfter(time.Second + time.Nanosecond); got != 26 {
		t.Fatalf("mid-frame subtitle start = %d, want 26", got)
	}
	if got := rate.FrameAt(time.Second + time.Nanosecond); got != 25 {
		t.Fatalf("containing-frame lookup = %d, want 25", got)
	}
}

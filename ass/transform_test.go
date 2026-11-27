package ass

import (
	"strings"
	"testing"
	"time"
)

func TestNestedTransformsBecomeStaticSegments(t *testing.T) {
	script, err := ParseString(nestedASS)
	if err != nil {
		t.Fatal(err)
	}
	cues, err := script.Segments(Options{SampleInterval: 40 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) < 40 {
		t.Fatalf("segments = %d, expected sampled transform intervals", len(cues))
	}

	start := cueAt(t, cues, time.Second)
	if start.State.ScaleX != 100 || start.State.ScaleY != 100 ||
		start.State.RotateZ != 0 || *start.State.Position != (Point{X: 320, Y: 180}) {
		t.Fatalf("initial state = %+v", start.State)
	}
	mid := cueAt(t, cues, 1480*time.Millisecond)
	if !almostEqual(mid.State.ScaleX, 148) || !almostEqual(mid.State.RotateZ, 43.2) ||
		!almostEqual(mid.State.Position.X, 473.6) || !almostEqual(mid.State.Position.Y, 266.4) {
		t.Fatalf("state at 1.48s = %+v", mid.State)
	}
	nestedMid := cueAt(t, cues, 2240*time.Millisecond)
	if !almostEqual(nestedMid.State.ScaleX, 164) || !almostEqual(nestedMid.State.RotateZ, 133.2) {
		t.Fatalf("nested transform state at 2.24s = %+v", nestedMid.State)
	}
	settled := cueAt(t, cues, 3480*time.Millisecond)
	if settled.State.ScaleX != 50 || settled.State.RotateZ != 180 ||
		*settled.State.Position != (Point{X: 640, Y: 360}) {
		t.Fatalf("settled state = %+v", settled.State)
	}
	if start.End != 1040*time.Millisecond {
		t.Fatalf("first segment ends at %s, want 1.04s", start.End)
	}
}

func TestWebVTTDoesNotApplyFinalStateToWholeCue(t *testing.T) {
	script, err := ParseString(nestedASS)
	if err != nil {
		t.Fatal(err)
	}
	output, err := script.WebVTT(Options{SampleInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output, "WEBVTT\n\n") {
		t.Fatalf("missing WEBVTT header: %q", output[:min(20, len(output))])
	}
	if strings.Contains(output, `\t(`) || strings.Contains(output, "{\\") {
		t.Fatal("ASS override syntax leaked into WebVTT")
	}
	if !strings.Contains(output, "Hello, world &lt;3 &amp; more") {
		t.Fatal("converted cue does not preserve text")
	}
	if strings.Contains(output, "{world}") || strings.Contains(output, "{\\") {
		t.Fatal("ASS override tags leaked into cue text")
	}

	first := strings.Index(output, "00:00:01.000 -->")
	if first < 0 {
		t.Fatal("first cue is missing")
	}
	firstEnd := strings.Index(output[first:], "\n\n")
	if firstEnd < 0 {
		t.Fatal("first cue is unterminated")
	}
	firstCue := output[first : first+firstEnd]
	if strings.Contains(firstCue, "scale(2") || strings.Contains(firstCue, "rotateZ(180") {
		t.Fatalf("final transform applied to first cue:\n%s", firstCue)
	}
	if !strings.Contains(firstCue, "position:50.000% line:50.000% align:center") {
		t.Fatalf("initial position setting missing:\n%s", firstCue)
	}
	if !strings.Contains(output, "scale(2.000, 2.000)") ||
		!strings.Contains(output, "rotateZ(180.000deg)") {
		t.Fatal("sampled final transform is missing from later cues")
	}
}

func TestStaticCueIsNotSplit(t *testing.T) {
	script, err := ParseString(nestedASS)
	if err != nil {
		t.Fatal(err)
	}
	cues, err := script.Segments(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, cue := range cues {
		if cue.Start >= 5*time.Second {
			if cue.End-cue.Start != 1500*time.Millisecond {
				t.Fatalf("static cue duration = %s, want 1.5s", cue.End-cue.Start)
			}
			return
		}
	}
	t.Fatal("static dialogue cue not found")
}

func cueAt(t *testing.T, cues []Cue, at time.Duration) Cue {
	t.Helper()
	for _, cue := range cues {
		if at >= cue.Start && at < cue.End {
			return cue
		}
	}
	t.Fatalf("no cue covers %s", at)
	return Cue{}
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func almostEqual(left, right float64) bool {
	const epsilon = 1e-9
	difference := left - right
	if difference < 0 {
		difference = -difference
	}
	return difference < epsilon
}

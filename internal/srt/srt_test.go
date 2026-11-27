package srt

import (
	"strings"
	"testing"
	"time"
)

func TestParseBasic(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:03,500\nHello\nworld\n\n" +
		"2\n00:00:04.000 --> 00:00:05,250 X1:0 Y1:20 X2:1280 Y2:120\nTop line\n"
	f, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Cues) != 2 {
		t.Fatalf("expected 2 cues, got %d", len(f.Cues))
	}
	c := f.Cues[0]
	if c.Start != time.Second || c.End != 3500*time.Millisecond {
		t.Fatalf("unexpected timing: %v -> %v", c.Start, c.End)
	}
	if len(c.Lines) != 2 || c.Lines[0] != "Hello" || c.Lines[1] != "world" {
		t.Fatalf("unexpected lines: %#v", c.Lines)
	}
	if c.Position.Has {
		t.Fatal("first cue should not carry coordinates")
	}

	c2 := f.Cues[1]
	if !c2.Position.Has || c2.Position.Y1 != 20 {
		t.Fatalf("coordinates not parsed: %#v", c2.Position)
	}
	if RegionFromPosition(c2.Position) != RegionTop {
		t.Fatal("low Y1 must classify as top region")
	}
}

func TestParseBOMAndCRLF(t *testing.T) {
	input := "\xef\xbb\xbf1\r\n00:00:01,000 --> 00:00:02,000\r\nHi\r\n"
	f, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Cues) != 1 || f.Cues[0].Lines[0] != "Hi" {
		t.Fatalf("BOM/CRLF parse failed: %#v", f.Cues)
	}
}

func TestParseRejectsReversedTiming(t *testing.T) {
	input := "1\n00:00:03,000 --> 00:00:02,000\nBad\n"
	if _, err := Parse(strings.NewReader(input)); err == nil {
		t.Fatal("expected error for end-before-start cue")
	}
}

func TestTimeRoundTrip(t *testing.T) {
	for _, tc := range []string{"00:00:00,000", "01:02:03,004", "99:59:59,999"} {
		d, err := ParseTime(tc)
		if err != nil {
			t.Fatal(err)
		}
		if got := FormatTime(d); got != tc {
			t.Fatalf("round trip %q -> %q", tc, got)
		}
	}
}

func TestRenderRegionSuffix(t *testing.T) {
	out := Render([]RenderCue{
		{Start: 0, End: 1000, Region: RegionTop, Lines: []string{"up"}},
		{Start: 1000, End: 2000, Region: RegionBottom, Lines: []string{"down"}},
	})
	if !strings.Contains(out, "00:00:00,000 --> 00:00:01,000 X1:0 Y1:20") {
		t.Fatalf("top cue missing coordinates:\n%s", out)
	}
	if strings.Count(out, "\n\n") != 1 {
		t.Fatalf("expected one blank separator between cues:\n%s", out)
	}
}

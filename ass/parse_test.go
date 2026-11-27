package ass

import (
	"strings"
	"testing"
	"time"
)

const nestedASS = `[Script Info]
Title: nested transform
PlayResX: 640
PlayResY: 360
ScriptType: v4.00+

[V4+ Styles]
Format: Name, Fontname, Fontsize
Style: Default,Arial,20

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:04.00,Default,,0,0,0,,{\fscx100\fscy100\frz0\pos(320,180)\t(0,1000,\fscx200\fscy200\frz90\pos(640,360))\t(1000,2000,\fscx50\t(1000,1500,\frz180))}Hello, world <3 & more
Dialogue: 1,0:00:05.00,0:00:06.50,Default,,0,0,0,,No animation
`

func TestParseASSFormatPlayResAndDialogue(t *testing.T) {
	script, err := ParseString(nestedASS)
	if err != nil {
		t.Fatal(err)
	}
	if script.Style.PlayResX != 640 || script.Style.PlayResY != 360 {
		t.Fatalf("play resolution = %dx%d", script.Style.PlayResX, script.Style.PlayResY)
	}
	if len(script.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(script.Events))
	}
	first := script.Events[0]
	if first.Start != time.Second || first.End != 4*time.Second {
		t.Fatalf("event interval = %s..%s", first.Start, first.End)
	}
	if !strings.Contains(first.Text, "Hello, world <3 & more") {
		t.Fatalf("dialogue text lost comma or content: %q", first.Text)
	}
}

func TestParseTimestampAndFormatTimestamp(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		vtt   string
	}{
		{"0:00:00.00", 0, "00:00:00.000"},
		{"0:01:02.34", time.Minute + 2*time.Second + 340*time.Millisecond, "00:01:02.340"},
		{"2:03:04.567", 2*time.Hour + 3*time.Minute + 4*time.Second + 567*time.Millisecond, "02:03:04.567"},
	}
	for _, test := range tests {
		got, err := ParseTimestamp(test.input)
		if err != nil {
			t.Fatalf("ParseTimestamp(%q): %v", test.input, err)
		}
		if got != test.want {
			t.Fatalf("ParseTimestamp(%q) = %s, want %s", test.input, got, test.want)
		}
		if formatted := FormatTimestamp(got); formatted != test.vtt {
			t.Fatalf("FormatTimestamp(%s) = %q, want %q", got, formatted, test.vtt)
		}
	}
}

func TestTagSequenceHandlesNestedCommas(t *testing.T) {
	line, err := parseLine(`{\t(0,500,\fscx200\clip(m 0,0 l 100,100))}clip`)
	if err != nil {
		t.Fatal(err)
	}
	if len(line.timeline.animations) != 1 {
		t.Fatalf("animations = %d, want 1", len(line.timeline.animations))
	}
	anim := line.timeline.animations[0]
	if anim.start != 0 || anim.end != 500*time.Millisecond {
		t.Fatalf("animation interval = %s..%s", anim.start, anim.end)
	}
	if got := anim.changes[propScaleX]; got != 200 {
		t.Fatalf("target scale = %v, want 200", got)
	}
}

func TestNestedTransitionKeepsBothTimeIntervals(t *testing.T) {
	line, err := parseLine(`{\t(0,1000,\fscx200\t(200,700,\frz90))}nested`)
	if err != nil {
		t.Fatal(err)
	}
	if len(line.timeline.animations) != 2 {
		t.Fatalf("animations = %d, want parent and nested transitions", len(line.timeline.animations))
	}
	var scale, rotation *animation
	for _, anim := range line.timeline.animations {
		if _, ok := anim.changes[propScaleX]; ok {
			scale = anim
		}
		if _, ok := anim.changes[propRotateZ]; ok {
			rotation = anim
		}
	}
	if scale == nil || scale.start != 0 || scale.end != time.Second {
		t.Fatalf("parent scale interval = %+v", scale)
	}
	if rotation == nil || rotation.start != 200*time.Millisecond || rotation.end != 700*time.Millisecond {
		t.Fatalf("nested rotation interval = %+v", rotation)
	}
}

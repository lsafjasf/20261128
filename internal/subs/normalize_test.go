package subs

import (
	"testing"
	"time"
)

func TestInferFrameRate(t *testing.T) {
	cases := []struct {
		name string
		ms   []int64
		want float64
	}{
		{"24fps grid", []int64{0, 1000, 2042, 3083}, 24},
		{"25fps grid", []int64{0, 1000, 2000, 3040, 4080}, 25},
		{"30fps grid", []int64{0, 1000, 2033, 3067}, 30},
		{"29.97 drop grid", []int64{133, 1001, 2002, 3003}, 29.97},
		{"wall clock only", []int64{137, 913, 1777}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := &Document{Format: "srt", Timebase: Timebase{Scale: "1000"}}
			for i := 0; i+1 < len(tc.ms); i += 2 {
				doc.Cues = append(doc.Cues, Cue{
					Start: time.Duration(tc.ms[i]) * time.Millisecond,
					End:   time.Duration(tc.ms[i+1]) * time.Millisecond,
				})
			}
			inferFrameRate(doc)
			if doc.Timebase.FrameRate != tc.want {
				t.Errorf("got %v, want %v", doc.Timebase.FrameRate, tc.want)
			}
		})
	}
}

func TestNormalizeText(t *testing.T) {
	raw := "<v Ann>Hello   world\r\n<i>line two</i>\r\n\r\n"
	want := "Hello world\nline two"
	if got := NormalizeText(raw); got != want {
		t.Errorf("NormalizeText() = %q, want %q", got, want)
	}
}

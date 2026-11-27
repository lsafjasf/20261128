package imscfit

import (
	"strings"
	"testing"
)

func parseString(s string) (*Document, error) {
	return Parse(strings.NewReader(s))
}

func TestParseClock(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"00:00:01.500", 1.5},
		{"01:02:03", 3723},
		{"5s", 5},
		{"250ms", 0.25},
		{"2m", 120},
		{"1h", 3600},
	}
	for _, tc := range cases {
		got, ok := parseClock(tc.in)
		if !ok || got != tc.want {
			t.Errorf("parseClock(%q) = %v,%v want %v", tc.in, got, ok, tc.want)
		}
	}
}

func TestForcedBreakAndSpans(t *testing.T) {
	src := `
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling">
  <head><layout>
    <region xml:id="r" tts:extent="50% 50%"/>
  </layout></head>
  <body><div>
    <p xml:id="p1" region="r" begin="1s" end="2s">
      first line<br/>second <span tts:fontSize="1.4em">big</span> word
    </p>
  </div></body>
</tt>`
	doc, err := parseString(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Paragraphs) != 1 {
		t.Fatalf("paragraphs = %d", len(doc.Paragraphs))
	}
	p := doc.Paragraphs[0]
	breakCount := 0
	for _, s := range p.Segments {
		if s.Break {
			breakCount++
		}
	}
	if breakCount != 1 {
		t.Errorf("breaks = %d, want 1", breakCount)
	}
	if p.BeginSec != 1 || p.EndSec != 2 {
		t.Errorf("timing = %.1f-%.1f, want 1-2", p.BeginSec, p.EndSec)
	}

	c := Canvas{Name: "c", Width: 1000, Height: 500}
	props := doc.EffectiveProps(doc.Regions["r"])
	g := doc.ResolveGeometry(doc.Regions["r"], props, c)
	if g.Width != 500 || g.Height != 250 {
		t.Errorf("region extent = %.0fx%.0f, want 500x250", g.Width, g.Height)
	}
	block := doc.LayOut(p, props, g, c)
	if len(block.Lines) < 2 {
		t.Errorf("expected forced break to create >=2 lines, got %d", len(block.Lines))
	}
}

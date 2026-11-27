package imscfit

import (
	"os"
	"testing"
)

func TestParseLength(t *testing.T) {
	cases := map[string]Length{
		"80%":    {Value: 80, Unit: UnitPercent, Set: true},
		"12c":    {Value: 12, Unit: UnitCell, Set: true},
		"1.5em":  {Value: 1.5, Unit: UnitEM, Set: true},
		"1920px": {Value: 1920, Unit: UnitPX, Set: true},
		"2":      {Value: 2, Unit: UnitEM, Set: true},
	}
	for in, want := range cases {
		got, err := ParseLength(in)
		if err != nil {
			t.Fatalf("ParseLength(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("ParseLength(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestResolveGeometryWithPadding(t *testing.T) {
	doc := newDocument()
	r := &Region{
		ID:     "r",
		Origin: LengthPair{mustLen("10%"), mustLen("80%")},
		Extent: LengthPair{mustLen("80%"), mustLen("15%")},
		Props:  Props{Padding: parsePadding("0.5c 1c")},
	}
	doc.Regions[r.ID] = r

	c := Canvas{Name: "hd", Width: 1920, Height: 1080}
	g := doc.ResolveGeometry(r, doc.EffectiveProps(r), c)

	if g.OriginX != 192 || g.OriginY != 864 {
		t.Errorf("origin = %.1f,%.1f, want 192,864", g.OriginX, g.OriginY)
	}
	if g.Width != 1536 || g.Height != 162 {
		t.Errorf("extent = %.1fx%.1f, want 1536x162", g.Width, g.Height)
	}
	// 1c horizontal = 60px, vertical = 72px.
	if g.PadLeft != 60 || g.PadRight != 60 || g.PadTop != 36 || g.PadBottom != 36 {
		t.Errorf("padding = T%.0f R%.0f B%.0f L%.0f", g.PadTop, g.PadRight, g.PadBottom, g.PadLeft)
	}
	if g.ContentW != 1416 || g.ContentH != 90 {
		t.Errorf("content = %.0fx%.0f, want 1416x90", g.ContentW, g.ContentH)
	}
}

func TestSpillDirectionByDisplayAlign(t *testing.T) {
	block := Block{UsedH: 300, OverflowY: 200}
	g := Geometry{ContentH: 100}

	top, bottom := spill(block, g, "before")
	if top != 0 || bottom != 200 {
		t.Errorf("before spill = %.0f/%.0f, want 0/200", top, bottom)
	}
	top, bottom = spill(block, g, "after")
	if top != 200 || bottom != 0 {
		t.Errorf("after spill = %.0f/%.0f, want 200/0", top, bottom)
	}
	top, bottom = spill(block, g, "center")
	if top != 100 || bottom != 100 {
		t.Errorf("center spill = %.0f/%.0f, want 100/100", top, bottom)
	}
}

func TestParseSampleEndToEnd(t *testing.T) {
	f, err := os.Open("testdata/sample.ttml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Regions) != 3 {
		t.Fatalf("regions = %d, want 3", len(doc.Regions))
	}
	if len(doc.Paragraphs) != 4 {
		t.Fatalf("paragraphs = %d, want 4", len(doc.Paragraphs))
	}
	if doc.CellX != 32 || doc.CellY != 15 {
		t.Errorf("cell resolution = %dx%d", doc.CellX, doc.CellY)
	}

	c := Canvas{Name: "hd", Width: 1920, Height: 1080}
	rep := doc.Analyze(c)
	status := map[string]string{}
	for _, cue := range rep.Cues {
		status[cue.ParagraphID] = cue.Status
	}
	if status["cue4"] != FitOK {
		t.Errorf("cue4 = %s, want ok", status["cue4"])
	}
	if status["cue1"] != FitClipped {
		t.Errorf("cue1 = %s, want clipped", status["cue1"])
	}
	if status["cue2"] != FitSpilling {
		t.Errorf("cue2 = %s, want spilling", status["cue2"])
	}

	p := doc.Paragraphs[0]
	cand := doc.BuildCandidates(p, c)
	if len(cand.ShortenText) == 0 {
		t.Error("expected shorten-text candidate for cue1")
	}
	if len(cand.EnlargeRegion) == 0 {
		t.Error("expected enlarge-region candidate for cue1")
	}
}

func TestNarrowCanvasWrapsMoreLines(t *testing.T) {
	f, _ := os.Open("testdata/sample.ttml")
	defer f.Close()
	doc, _ := Parse(f)

	wide := doc.Analyze(Canvas{Name: "wide", Width: 1920, Height: 1080})
	narrow := doc.Analyze(Canvas{Name: "4:3", Width: 1440, Height: 1080})
	if wide.Cues[0].LineCount >= narrow.Cues[0].LineCount {
		t.Errorf("narrow canvas should wrap more lines: %d vs %d",
			narrow.Cues[0].LineCount, wide.Cues[0].LineCount)
	}
}

func mustLen(s string) Length {
	l, err := ParseLength(s)
	if err != nil {
		panic(err)
	}
	return l
}

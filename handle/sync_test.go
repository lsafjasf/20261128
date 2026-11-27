package handle

import "testing"

func TestRelayoutAndAdjust(t *testing.T) {
	p, err := Plan(60_000, 10_000, 50_000, 2_000)
	if err != nil {
		t.Fatal(err)
	}
	cues := []Cue{
		{Start: 5_000, End: 5_900, Text: "before window"},  // dropped
		{Start: 8_500, End: 12_000, Text: "crosses start"}, // clamped to 0
		{Start: 20_000, End: 22_000, Text: "inside"},
		{Start: 51_000, End: 53_000, Text: "crosses end"}, // clamped to 52s
	}
	tracks := []AudioTrack{
		{SourceStart: 5_000, Duration: 10_000}, // starts before window
		{SourceStart: 30_000, Duration: 5_000}, // fully inside
		{SourceStart: 100, Duration: 1_000},    // ends before window
	}

	l := Relayout(p, cues, tracks)
	if len(l.Cues) != 3 {
		t.Fatalf("cues=%d", len(l.Cues))
	}
	if l.Cues[0].Offset != 500 || l.Cues[0].Start != 8_500 {
		t.Fatalf("cue0 offset=%d start=%d", l.Cues[0].Offset, l.Cues[0].Start)
	}
	if l.Cues[1].Offset != 12_000 {
		t.Fatalf("cue1 offset=%d", l.Cues[1].Offset)
	}
	if l.Cues[2].End != 52_000 {
		t.Fatalf("cue2 end=%d", l.Cues[2].End)
	}
	if len(l.Audio) != 2 {
		t.Fatalf("audio=%d", len(l.Audio))
	}
	// Track 0: 3s trimmed off the head, lands at timeline 0.
	if l.Audio[0].TrimHead != 3_000 || l.Audio[0].Offset != 0 {
		t.Fatalf("audio0 trim=%d offset=%d", l.Audio[0].TrimHead, l.Audio[0].Offset)
	}
	if l.Audio[1].Offset != 22_000 || l.Audio[1].TrimHead != 0 {
		t.Fatalf("audio1 offset=%d trim=%d", l.Audio[1].Offset, l.Audio[1].TrimHead)
	}

	// User drags the in-point 2s earlier: window grows, offsets shift.
	l2, err := Adjust(p, 8_000, 50_000, cues, tracks)
	if err != nil {
		t.Fatal(err)
	}
	if l2.Plan.ExportStart != 6_000 {
		t.Fatalf("export start=%d", l2.Plan.ExportStart)
	}
	// "crosses start" cue now starts at 8.5s source -> 2.5s on timeline.
	if l2.Cues[0].Offset != 2_500 {
		t.Fatalf("shifted cue0 offset=%d", l2.Cues[0].Offset)
	}
	// Track 0 now needs only 1s trimmed.
	if l2.Audio[0].TrimHead != 1_000 {
		t.Fatalf("shifted audio0 trim=%d", l2.Audio[0].TrimHead)
	}
}

func TestAdjustInvalidKeepsOldPlan(t *testing.T) {
	p, err := Plan(60_000, 10_000, 50_000, 2_000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Adjust(p, 55_000, 50_000, nil, nil); err == nil {
		t.Fatal("want error for inverted range")
	}
}

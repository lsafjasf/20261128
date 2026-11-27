package plan

import (
	"strings"
	"testing"

	"srtplan/internal/srt"
)

func parseFile(_ *testing.T, content string) *srt.File {
	f, err := srt.Parse(strings.NewReader(content))
	if err != nil {
		panic("parse fixture: " + err.Error() + "\n" + content)
	}
	return f
}

func twoTrackInputs(primary, secondary string) []Input {
	return []Input{
		{Name: "primary", Priority: 10, Primary: true, File: parseFile(nil, primary)},
		{Name: "secondary", Priority: 5, File: parseFile(nil, secondary)},
	}
}

func TestBilingualOverlapUsesRegionsNotConcatenation(t *testing.T) {
	primary := "1\n00:00:01,000 --> 00:00:04,000\n你好，世界\n"
	secondary := "1\n00:00:01,500 --> 00:00:03,500\nHello world\n"

	report, err := Plan(twoTrackInputs(primary, secondary), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Stats.ContentCues != 2 || report.Stats.DeferredItems != 0 {
		t.Fatalf("expected both cues placed without deferral: %+v", report.Stats)
	}
	regions := map[string]string{}
	for _, item := range report.Items {
		regions[item.Source] = item.Region
		if strings.Contains(strings.Join(item.Lines, " "), "你好") &&
			strings.Contains(strings.Join(item.Lines, " "), "Hello") {
			t.Fatal("bilingual text must never be concatenated into one item")
		}
	}
	if regions["primary"] != RegionBottom || regions["secondary"] != RegionTop {
		t.Fatalf("unexpected regions: %#v", regions)
	}
	for _, cov := range report.Coverage {
		if !cov.Complete {
			t.Fatalf("coverage incomplete: %+v", cov)
		}
	}
}

func TestThreeWayOverlapSplitsAndDefersWithoutDropping(t *testing.T) {
	primary := "1\n00:00:00,000 --> 00:00:10,000\nALICE: 你好\n"
	secondary := "1\n00:00:02,000 --> 00:00:08,000\nHello there\n"
	tertiary := parseFile(t, "1\n00:00:04,000 --> 00:00:06,000\n第三轨字幕\n")

	inputs := twoTrackInputs(primary, secondary)
	inputs = append(inputs, Input{Name: "tertiary", Priority: 1, File: tertiary})
	report, err := Plan(inputs, Options{})
	if err != nil {
		t.Fatal(err)
	}

	if report.Stats.DroppedCues != 0 {
		t.Fatal("no cue may be dropped")
	}
	var tertiaryItems []Item
	for _, item := range report.Items {
		if item.Source == "tertiary" {
			tertiaryItems = append(tertiaryItems, item)
		}
	}
	if len(tertiaryItems) == 0 {
		t.Fatal("tertiary cue vanished despite losing region contention")
	}
	for _, item := range tertiaryItems {
		if !item.Deferred || item.StartMs < 6000 {
			t.Fatalf("tertiary fragment must be deferred past the conflict: %+v", item)
		}
		if strings.Join(item.Lines, "") != "第三轨字幕" {
			t.Fatalf("deferred cue lost its text: %#v", item.Lines)
		}
	}

	alice := findItems(report, "primary#1")
	if len(alice) == 0 || alice[0].Speaker != "ALICE" {
		t.Fatalf("speaker label not extracted: %+v", alice)
	}
	for _, cov := range report.Coverage {
		if !cov.Complete {
			t.Fatalf("coverage incomplete for %s: %+v", cov.CueID, cov)
		}
	}
}

func TestManualMarkerWinsAndIsExplained(t *testing.T) {
	primary := "1\n00:00:00,000 --> 00:00:05,000\n自动字幕\n"
	secondary := "1\n00:00:00,000 --> 00:00:05,000\n[manual:9] [bottom] 人工校对字幕\n"

	report, err := Plan(twoTrackInputs(primary, secondary), Options{})
	if err != nil {
		t.Fatal(err)
	}
	var manual, auto Item
	for _, item := range report.Items {
		if item.Manual {
			manual = item
		} else {
			auto = item
		}
	}
	if manual.Region != RegionBottom {
		t.Fatalf("manual cue should win the bottom region: %+v", manual)
	}
	if auto.Region != RegionTop || auto.Deferred {
		t.Fatalf("automatic cue should be moved to top, not deferred: %+v", auto)
	}
	if manual.Decision != DecisionManualPlaced {
		t.Fatalf("unexpected decision %q", manual.Decision)
	}
	if !containsReason(manual.Reasons, "manual marker") {
		t.Fatalf("manual placement not explained: %#v", manual.Reasons)
	}
}

func TestSameTrackContentionSplitsLoser(t *testing.T) {
	first := "1\n00:00:00,000 --> 00:00:10,000\nFirst long cue\n"
	bottom := parseFile(t, "1\n00:00:04,000 --> 00:00:06,000\n[manual] [bottom] Bottom overlay\n")
	top := parseFile(t, "1\n00:00:04,000 --> 00:00:06,000\n[top] Top overlay\n")

	inputs := []Input{
		{Name: "primary", Priority: 10, Primary: true, File: parseFile(t, first)},
		{Name: "overlay-bottom", Priority: 10, File: bottom},
		{Name: "overlay-top", Priority: 9, File: top},
	}
	report, err := Plan(inputs, Options{})
	if err != nil {
		t.Fatal(err)
	}
	loser := findItems(report, "primary#1")
	if len(loser) < 2 {
		t.Fatalf("first cue should be split around the two overlays: %+v", loser)
	}
	hasDeferred := false
	for _, item := range loser {
		if !item.Split {
			t.Fatalf("every fragment of a split cue must be marked split: %+v", item)
		}
		if item.Deferred {
			hasDeferred = true
		}
	}
	if !hasDeferred {
		t.Fatalf("middle fragment should be deferred while outer fragments stay placed: %+v", loser)
	}
	for _, cov := range report.Coverage {
		if !cov.Complete {
			t.Fatalf("coverage incomplete: %+v", cov)
		}
	}
}

func TestEmptyCueIsNotScheduled(t *testing.T) {
	primary := "1\n00:00:00,000 --> 00:00:02,000\n\n\n2\n00:00:02,000 --> 00:00:04,000\n有内容\n"
	report, err := Plan(twoTrackInputs(primary, "1\n00:00:00,000 --> 00:00:01,000\ntranslation\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Stats.ContentCues != 2 {
		t.Fatalf("only text-bearing cues count as content, got %+v", report.Stats)
	}
	for _, item := range report.Items {
		if item.CueID == "primary#1" {
			t.Fatal("empty cue must not produce an item")
		}
	}
}

func TestSpeakerLabelsDoNotEatNormalSentences(t *testing.T) {
	cases := map[string]string{
		"ALICE: 你好":                       "ALICE",
		"NEWS ANCHOR: Breaking news":      "NEWS ANCHOR",
		"Alice (V.O.): Over here":         "Alice (V.O.)",
		"[Narrator]: It was a dark night": "Narrator",
		"Third track: extra annotation":   "",
		"Hello: welcome to the show":      "",
	}
	for text, want := range cases {
		cue := enrich("x#1", 0, srt.Cue{
			Start: 0, End: 1_000_000_000,
			Lines: []string{text},
		}, Input{Name: "x"}, Options{})
		if cue.Speaker != want {
			t.Errorf("speaker for %q = %q, want %q", text, cue.Speaker, want)
		}
		if want != "" && !strings.HasPrefix(cue.DisplayText, strings.SplitN(text, ":", 2)[1][1:]) {
			t.Errorf("display text should drop the label for %q, got %q", text, cue.DisplayText)
		}
	}
}

func findCue(t *testing.T, report *Report, id string) Coverage {
	t.Helper()
	for _, cov := range report.Coverage {
		if cov.CueID == id {
			return cov
		}
	}
	t.Fatalf("cue %s missing from coverage", id)
	return Coverage{}
}

func findItems(report *Report, id string) []Item {
	var out []Item
	for _, item := range report.Items {
		if item.CueID == id {
			out = append(out, item)
		}
	}
	return out
}

func containsReason(reasons []string, fragment string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, fragment) {
			return true
		}
	}
	return false
}

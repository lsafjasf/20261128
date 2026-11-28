package subs

import "testing"

func TestLanguageFromName(t *testing.T) {
	cases := map[string]string{
		"release_track01_en.srt": "en",
		"track02_zh-Hans.srt":    "zh-Hans",
		"movie.fr.ass":           "fr",
		"final_cut_eng.vtt":      "en", // ISO 639-3 "eng" normalizes to "en"
		"clip.pt-BR.srt":         "pt-BR",
		"no_lang_here.srt":       "",
	}
	for name, want := range cases {
		if got := LanguageFromName(name); got != want {
			t.Errorf("LanguageFromName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestParseSRT(t *testing.T) {
	doc, err := Parse("../../testdata/embedded/release_track01_en.srt")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Language != "en" {
		t.Errorf("language = %q, want en", doc.Language)
	}
	if doc.CueCount() != 3 {
		t.Fatalf("cues = %d, want 3", doc.CueCount())
	}
	if got := doc.Cues[2].Text; got != "Thanks for watching." {
		t.Errorf("cue text = %q", got)
	}
	if doc.Timebase.FrameRate != 24 {
		t.Errorf("frame rate = %v, want 24 (smallest fitting grid for these timestamps)", doc.Timebase.FrameRate)
	}
}

func TestParseVTTShortClocks(t *testing.T) {
	doc, err := Parse("../../testdata/sidecar/final_cut_eng.vtt")
	if err != nil {
		t.Fatal(err)
	}
	if doc.CueCount() != 3 {
		t.Fatalf("cues = %d, want 3", doc.CueCount())
	}
	if doc.Cues[0].Start.Milliseconds() != 1000 {
		t.Errorf("first start = %v, want 1s", doc.Cues[0].Start)
	}
}

func TestParseASS(t *testing.T) {
	doc, err := Parse("../../testdata/sidecar/zh_hans_v2_renamed.ass")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Language != "zh-Hans" {
		t.Errorf("language = %q, want zh-Hans", doc.Language)
	}
	if doc.CueCount() != 3 {
		t.Fatalf("cues = %d, want 3", doc.CueCount())
	}
	for _, cue := range doc.Cues {
		for _, r := range cue.Text {
			if r == '{' || r == '}' || r == '\\' {
				t.Fatalf("uncleaned override tag in %q", cue.Text)
			}
		}
	}
	if doc.Timebase.Scale != "100" {
		t.Errorf("scale = %q, want 100", doc.Timebase.Scale)
	}
}

func TestCrossFormatContentEquality(t *testing.T) {
	srt, err := Parse("../../testdata/embedded/release_track01_en.srt")
	if err != nil {
		t.Fatal(err)
	}
	vtt, err := Parse("../../testdata/sidecar/final_cut_eng.vtt")
	if err != nil {
		t.Fatal(err)
	}
	if srt.ContentHash() != vtt.ContentHash() {
		t.Error("same translation in SRT and VTT must hash equal")
	}
	if srt.TimelineHash() != vtt.TimelineHash() {
		t.Error("identical timings must hash equal")
	}
}

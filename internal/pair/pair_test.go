package pair

import (
	"testing"
	"time"

	"subaudit/internal/subs"
)

func doc(path, lang, format, scale string, fps float64, texts []string, starts []time.Duration) *subs.Document {
	d := &subs.Document{
		Path:     path,
		Format:   format,
		Language: lang,
		Timebase: subs.Timebase{Scale: scale, FrameRate: fps},
	}
	for i, text := range texts {
		start := starts[i]
		d.Cues = append(d.Cues, subs.Cue{Start: start, End: start + 3*time.Second, Text: text})
	}
	return d
}

func sec(n int) time.Duration { return time.Duration(n) * time.Second }

func TestVerdicts(t *testing.T) {
	texts := []string{"hello", "world", "bye"}
	starts := []time.Duration{sec(1), sec(5), sec(9)}

	emb := []*subs.Document{
		doc("emb/en.srt", "en", "srt", "1000", 24, texts, starts),
		doc("emb/zh.srt", "zh", "srt", "1000", 24, texts, starts),
		doc("emb/fr.srt", "fr", "srt", "1000", 24, texts, starts),
		doc("emb/de.srt", "de", "srt", "1000", 24, texts, starts),
	}
	side := []*subs.Document{
		// en: identical content, different container, same timebase => SAME
		doc("side/different_name_en.vtt", "en", "vtt", "1000", 24,
			[]string{"hello", "world", "bye"}, starts),
		// zh: same language/cue count/timebase, revised translation => PARTIAL
		doc("side/zh.ass", "zh", "ass", "100", 0,
			[]string{"hello", "world", "bye"}, starts),
		// fr: nothing in common but language => MISMATCH
		doc("side/fr.srt", "fr", "srt", "1000", 25,
			[]string{"a", "b"}, []time.Duration{sec(20), sec(40)}),
		// ja: no embedded counterpart => unpaired sidecar
		doc("side/ja.srt", "ja", "srt", "1000", 24,
			[]string{"x"}, []time.Duration{sec(2)}),
	}

	rep := Audit(emb, side)
	got := map[string]Verdict{}
	for _, p := range rep.Pairs {
		got[p.Language] = p.Verdict
	}
	want := map[string]Verdict{
		"en": VerdictSame,
		"zh": VerdictPartial,
		"fr": VerdictMismatch,
	}
	for lang, v := range want {
		if got[lang] != v {
			t.Errorf("lang %s = %v, want %v", lang, got[lang], v)
		}
	}
	if len(rep.Pairs) != 3 {
		t.Fatalf("pairs = %d, want 3", len(rep.Pairs))
	}
	if len(rep.UnpairedEmbedded) != 1 || rep.UnpairedEmbedded[0].Language != "de" {
		t.Errorf("unpaired embedded = %+v", rep.UnpairedEmbedded)
	}
	if len(rep.UnpairedSidecar) != 1 || rep.UnpairedSidecar[0].Language != "ja" {
		t.Errorf("unpaired sidecar = %+v", rep.UnpairedSidecar)
	}
	if rep.Summary.Same != 1 || rep.Summary.PartialChange != 1 ||
		rep.Summary.Mismatch != 1 || rep.Summary.Unpaired != 2 {
		t.Errorf("summary = %+v", rep.Summary)
	}

	// Filename similarity must not influence the en match score.
	for _, p := range rep.Pairs {
		if p.Language == "en" && p.Score < 200 {
			t.Errorf("en pair score = %d, expected full fingerprint overlap", p.Score)
		}
	}
}

func TestFrameRateTolerance(t *testing.T) {
	a := doc("a.srt", "en", "srt", "1000", 23.976, []string{"x"}, []time.Duration{sec(1)})
	b := doc("b.srt", "en", "srt", "1000", 24.0, []string{"x"}, []time.Duration{sec(1)})
	if sameTimebase(a, b) {
		t.Error("23.976 fps and 24 fps must compare as different timebases")
	}
}

// Package pair matches embedded subtitle tracks against sidecar files by
// language and content/timeline fingerprints rather than filenames.
package pair

import (
	"fmt"
	"sort"

	"subaudit/internal/subs"
)

// Verdict classifies a matched embedded/sidecar pair.
type Verdict string

const (
	// VerdictSame: language, content hash, timebase and cue count all agree.
	VerdictSame Verdict = "SAME"
	// VerdictPartial: same language and clearly paired, but at least one
	// dimension differs while content lineage is still recognizable.
	VerdictPartial Verdict = "PARTIAL_CHANGE"
	// VerdictMismatch: paired only by language group; every substantive
	// fingerprint disagrees.
	VerdictMismatch Verdict = "MISMATCH"
)

// Side captures one endpoint of a comparison.
type Side struct {
	Path          string  `json:"path"`
	Format        string  `json:"format"`
	Language      string  `json:"language"`
	ContentHash   string  `json:"contentHash"`
	TimelineHash  string  `json:"timelineHash"`
	TimebaseScale string  `json:"timebaseScale"`
	FrameRate     float64 `json:"frameRate"`
	TimebaseSrc   string  `json:"timebaseSource"`
	CueCount      int     `json:"cueCount"`
}

// Dimension reports a single comparison axis.
type Dimension struct {
	Name   string `json:"name"`
	Equal  bool   `json:"equal"`
	Detail string `json:"detail"`
}

// Pair is one embedded/sidecar comparison result.
type Pair struct {
	Embedded Side        `json:"embedded"`
	Sidecar  Side        `json:"sidecar"`
	Language string      `json:"language"`
	Verdict  Verdict     `json:"verdict"`
	Score    int         `json:"matchScore"`
	Reasons  []string    `json:"reasons"`
	Dims     []Dimension `json:"dimensions"`
}

// Report is the full audit output.
type Report struct {
	Pairs            []Pair `json:"pairs"`
	UnpairedEmbedded []Side `json:"unpairedEmbedded"`
	UnpairedSidecar  []Side `json:"unpairedSidecars"`
	Summary          Counts `json:"summary"`
}

// Counts summarizes verdicts.
type Counts struct {
	Same          int `json:"same"`
	PartialChange int `json:"partialChange"`
	Mismatch      int `json:"mismatch"`
	Unpaired      int `json:"unpaired"`
}

func toSide(d *subs.Document) Side {
	return Side{
		Path:          d.Path,
		Format:        d.Format,
		Language:      d.Language,
		ContentHash:   d.ContentHash(),
		TimelineHash:  d.TimelineHash(),
		TimebaseScale: d.Timebase.Scale,
		FrameRate:     d.Timebase.FrameRate,
		TimebaseSrc:   d.Timebase.Source,
		CueCount:      d.CueCount(),
	}
}

// Audit pairs two document sets and classifies every match.
func Audit(embedded, sidecars []*subs.Document) *Report {
	rep := &Report{}
	embByLang := groupByLang(embedded)
	sideByLang := groupByLang(sidecars)

	langs := make(map[string]struct{}, len(embByLang)+len(sideByLang))
	for l := range embByLang {
		langs[l] = struct{}{}
	}
	for l := range sideByLang {
		langs[l] = struct{}{}
	}
	orderedLangs := make([]string, 0, len(langs))
	for l := range langs {
		orderedLangs = append(orderedLangs, l)
	}
	sort.Strings(orderedLangs)

	for _, lang := range orderedLangs {
		embs := embByLang[lang]
		sides := sideByLang[lang]
		usedEmb, usedSide := matchWithinLang(embs, sides, rep, lang)
		for i, d := range embs {
			if !usedEmb[i] {
				rep.UnpairedEmbedded = append(rep.UnpairedEmbedded, toSide(d))
			}
		}
		for i, d := range sides {
			if !usedSide[i] {
				rep.UnpairedSidecar = append(rep.UnpairedSidecar, toSide(d))
			}
		}
	}

	for _, p := range rep.Pairs {
		switch p.Verdict {
		case VerdictSame:
			rep.Summary.Same++
		case VerdictPartial:
			rep.Summary.PartialChange++
		case VerdictMismatch:
			rep.Summary.Mismatch++
		}
	}
	rep.Summary.Unpaired = len(rep.UnpairedEmbedded) + len(rep.UnpairedSidecar)
	sort.Slice(rep.Pairs, func(i, j int) bool {
		if rep.Pairs[i].Language != rep.Pairs[j].Language {
			return rep.Pairs[i].Language < rep.Pairs[j].Language
		}
		return rep.Pairs[i].Embedded.Path < rep.Pairs[j].Embedded.Path
	})
	return rep
}

func groupByLang(docs []*subs.Document) map[string][]*subs.Document {
	out := map[string][]*subs.Document{}
	for _, d := range docs {
		lang := d.Language
		if lang == "" {
			lang = "(unknown)"
		}
		out[lang] = append(out[lang], d)
	}
	return out
}

// score rewards shared fingerprints; filename never contributes.
func score(a, b *subs.Document) int {
	s := 0
	if a.ContentHash() == b.ContentHash() {
		s += 100
	}
	if a.TimelineHash() == b.TimelineHash() {
		s += 60
	}
	if a.CueCount() == b.CueCount() {
		s += 20
	}
	if sameTimebase(a, b) {
		s += 40
	}
	return s
}

func sameTimebase(a, b *subs.Document) bool {
	return a.Timebase.Scale == b.Timebase.Scale &&
		frameEqual(a.Timebase.FrameRate, b.Timebase.FrameRate)
}

func frameEqual(a, b float64) bool {
	const eps = 0.01
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}

// matchWithinLang greedily takes the highest-scoring available pairing until
// one side is exhausted or remaining scores show no shared fingerprint.
func matchWithinLang(embs, sides []*subs.Document, rep *Report, lang string) ([]bool, []bool) {
	usedEmb := make([]bool, len(embs))
	usedSide := make([]bool, len(sides))
	type cand struct{ i, j, s int }
	var cands []cand
	for i, e := range embs {
		for j, s := range sides {
			cands = append(cands, cand{i, j, score(e, s)})
		}
	}
	sort.Slice(cands, func(a, b int) bool { return cands[a].s > cands[b].s })
	for _, c := range cands {
		if usedEmb[c.i] || usedSide[c.j] {
			continue
		}
		// Same language guarantees a candidate pair. A score of zero means
		// no fingerprint overlaps and classify() will mark it MISMATCH;
		// only tracks whose language has no counterpart stay unpaired.
		usedEmb[c.i] = true
		usedSide[c.j] = true
		rep.Pairs = append(rep.Pairs, classify(embs[c.i], sides[c.j], lang, c.s))
	}
	return usedEmb, usedSide
}

func classify(emb, side *subs.Document, lang string, sc int) Pair {
	contentEq := emb.ContentHash() == side.ContentHash()
	timelineEq := emb.TimelineHash() == side.TimelineHash()
	cueEq := emb.CueCount() == side.CueCount()
	tbEq := sameTimebase(emb, side)

	dims := []Dimension{
		{Name: "language", Equal: true, Detail: lang},
		{Name: "content_hash", Equal: contentEq,
			Detail: fmt.Sprintf("emb=%s side=%s", short(emb.ContentHash()), short(side.ContentHash()))},
		{Name: "timebase", Equal: tbEq,
			Detail: fmt.Sprintf("emb=%s/%s side=%s/%s",
				emb.Timebase.Scale, fpsLabel(emb.Timebase.FrameRate),
				side.Timebase.Scale, fpsLabel(side.Timebase.FrameRate))},
		{Name: "cue_count", Equal: cueEq,
			Detail: fmt.Sprintf("emb=%d side=%d", emb.CueCount(), side.CueCount())},
		{Name: "timeline_hash", Equal: timelineEq,
			Detail: fmt.Sprintf("emb=%s side=%s", short(emb.TimelineHash()), short(side.TimelineHash()))},
	}

	var reasons []string
	add := func(ok bool, msg string) {
		if !ok {
			reasons = append(reasons, msg)
		}
	}
	add(contentEq, "content hash differs (different translation/revision)")
	add(tbEq, "timebase/frame rate differs")
	add(cueEq, fmt.Sprintf("cue count differs (%d vs %d)", emb.CueCount(), side.CueCount()))
	add(timelineEq, "cue timing differs")

	verdict := VerdictMismatch
	switch {
	case contentEq && tbEq && cueEq && timelineEq:
		verdict = VerdictSame
	case contentEq && tbEq && cueEq:
		verdict = VerdictSame
	case contentEq || timelineEq || (cueEq && tbEq):
		verdict = VerdictPartial
	default:
		verdict = VerdictMismatch
	}
	return Pair{
		Embedded: toSide(emb),
		Sidecar:  toSide(side),
		Language: lang,
		Verdict:  verdict,
		Score:    sc,
		Reasons:  reasons,
		Dims:     dims,
	}
}

func short(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}

func fpsLabel(fps float64) string {
	if fps == 0 {
		return "wallclock"
	}
	return fmt.Sprintf("%.3ffps", fps)
}

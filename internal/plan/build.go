package plan

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"srtplan/internal/language"
	"srtplan/internal/srt"
)

var (
	// [manual] and [manual:3] style markers; the optional number is priority.
	manualRe = regexp.MustCompile(`(?i)\[\s*manual(?:\s*:\s*(\d+))?\s*\]`)
	// Explicit region hints such as [top] / [bottom].
	regionHintRe = regexp.MustCompile(`(?i)\[\s*(top|bottom)\s*\]`)
	// Speaker labels must look like a label, not a normal sentence: either an
	// all-caps tag ("ALICE:", "NEWS ANCHOR:"), a short capitalised role with a
	// parenthetical ("Alice (V.O.):"), or a bracketed tag ("[Alice]:").
	speakerCapsRe    = regexp.MustCompile(`^\s*([A-Z][A-Z0-9 .'\-]{0,30})\s*[:：]\s*`)
	speakerRoleRe    = regexp.MustCompile(`^\s*([A-Z][a-z]+(?:\s+[A-Z][a-z]+){0,2}\s*\([^)]*\))\s*[:：]\s*`)
	speakerBracketRe = regexp.MustCompile(`^\s*\[\s*([A-Za-z][A-Za-z0-9 .'\-]{0,30})\s*\]\s*[:：]\s*`)
	// HTML/ASS style tags are stripped only for classification; rendering keeps
	// the cleaned text because coordinate regions carry the placement.
	tagRe = regexp.MustCompile(`<[^>]+>|\{[^}]+\}`)
)

// Build ingests all inputs and returns the enriched cues plus source metadata.
func Build(inputs []Input, opts Options) ([]*Cue, []SourceInfo, error) {
	if len(inputs) == 0 {
		return nil, nil, fmt.Errorf("plan: at least one SRT input is required")
	}
	if opts.CanvasY <= 0 {
		opts.CanvasY = 480
	}

	var cues []*Cue
	var sources []SourceInfo
	seenIDs := map[string]bool{}

	for si, in := range inputs {
		if in.File == nil {
			return nil, nil, fmt.Errorf("plan: input %q has no parsed file", in.Name)
		}
		info := SourceInfo{Name: in.Name, Priority: in.Priority, Primary: in.Primary}
		for _, raw := range in.File.Cues {
			id := fmt.Sprintf("%s#%d", in.Name, raw.Seq)
			if seenIDs[id] {
				return nil, nil, fmt.Errorf("plan: duplicate cue id %q", id)
			}
			seenIDs[id] = true

			cue := enrich(id, si, raw, in, opts)
			cues = append(cues, cue)
			if strings.TrimSpace(cue.DisplayText) != "" {
				info.CueCount++
			}
		}
		sources = append(sources, info)
	}
	return cues, sources, nil
}

func enrich(id string, sourceIndex int, raw srt.Cue, in Input, opts Options) *Cue {
	joined := strings.Join(raw.Lines, "\n")
	cleaned := strings.TrimSpace(tagRe.ReplaceAllString(joined, ""))

	manual := false
	manualPriority := 0
	cleaned = manualRe.ReplaceAllStringFunc(cleaned, func(match string) string {
		manual = true
		if m := manualRe.FindStringSubmatch(match); m[1] != "" {
			manualPriority, _ = strconv.Atoi(m[1])
		}
		return ""
	})

	preferred := ""
	cleaned = regionHintRe.ReplaceAllStringFunc(cleaned, func(match string) string {
		m := regionHintRe.FindStringSubmatch(match)
		preferred = strings.ToLower(m[1])
		return ""
	})

	speaker := ""
	firstLine := strings.SplitN(cleaned, "\n", 2)[0]
	if m, label := matchSpeaker(firstLine); m != nil {
		speaker = label
		cleaned = strings.TrimSpace(cleaned[len(m[0]):])
	}

	cleaned = collapseBlankLines(cleaned)
	region := srt.RegionFromPosition(raw.Position)
	if preferred == "" && region != srt.RegionUnspecified {
		preferred = region.String()
	}

	return &Cue{
		ID:              id,
		Source:          in.Name,
		SourceIndex:     sourceIndex,
		SourceSeq:       raw.Seq,
		StartMs:         ms(raw.Start),
		EndMs:           ms(raw.End),
		RawText:         joined,
		DisplayText:     cleaned,
		Speaker:         speaker,
		Language:        language.Detect(cleaned),
		Manual:          manual,
		ManualPriority:  manualPriority,
		PreferredRegion: preferred,
		Positioned:      raw.Position.Has,
	}
}

func matchSpeaker(line string) ([]string, string) {
	if m := speakerCapsRe.FindStringSubmatch(line); m != nil {
		return m, strings.TrimSpace(m[1])
	}
	if m := speakerRoleRe.FindStringSubmatch(line); m != nil {
		return m, strings.TrimSpace(m[1])
	}
	if m := speakerBracketRe.FindStringSubmatch(line); m != nil {
		return m, strings.TrimSpace(m[1])
	}
	return nil, ""
}

func collapseBlankLines(text string) string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

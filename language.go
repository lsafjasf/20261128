package webvtt

import (
	"sort"
	"strings"
)

// RenderOptions controls conversion of cue payload nodes to display text.
//
// Unknown tags are preserved by default. Set OmitUnknown to remove them.
// Timestamp tags are removed unless KeepTimestamps is set.
type RenderOptions struct {
	Language       string
	OmitUnknown    bool
	KeepTimestamps bool
}

// RenderResult is the display text and language-selection metadata produced by
// Render.
type RenderResult struct {
	Text              string
	RequestedLanguage string
	Language          string
	Fallback          bool
	Voices            []string
}

// Render converts a cue payload tree to display text.
//
// Language matching is case-insensitive and follows BCP 47 prefix fallback:
// en-US falls back to en, and a preference for en also accepts en-US. Untagged
// text is treated as base text and is retained. If no language tag matches, all
// text is returned and Fallback is true so a caller does not render an empty
// subtitle.
func Render(nodes []Node, opts RenderOptions) RenderResult {
	runs := collectRuns(nodes, "", "", opts)
	result := RenderResult{
		RequestedLanguage: opts.Language,
		Voices:            runVoices(runs),
	}
	if len(runs) == 0 {
		return result
	}
	if strings.TrimSpace(opts.Language) == "" {
		result.Text = joinRuns(runs, "", 0)
		return result
	}

	bestScore := 0
	bestLanguage := ""
	for _, run := range runs {
		score := languageMatchScore(opts.Language, run.language)
		if score > bestScore {
			bestScore = score
			bestLanguage = run.language
		}
	}
	if bestScore > 0 {
		result.Language = bestLanguage
		result.Text = joinRuns(runs, opts.Language, bestScore)
		return result
	}

	result.Fallback = true
	if first := firstLanguage(runs); first != "" {
		result.Language = first
	}
	result.Text = joinRuns(runs, "", 0)
	return result
}

// RenderText is the text-only form of Render.
func RenderText(nodes []Node, opts RenderOptions) string {
	return Render(nodes, opts).Text
}

// RenderCue renders c for opts.Language.
func RenderCue(c Cue, opts RenderOptions) RenderResult {
	c = parsedCue(c)
	return Render(c.Nodes, opts)
}

type textRun struct {
	text     string
	language string
	voice    string
}

func collectRuns(nodes []Node, language, voice string, opts RenderOptions) []textRun {
	var runs []textRun
	for _, node := range nodes {
		nodeLanguage := language
		if node.Type == NodeLanguage && node.Language != "" {
			nodeLanguage = node.Language
		}
		nodeVoice := voice
		if node.Type == NodeVoice && node.Voice != "" {
			nodeVoice = node.Voice
		}
		switch node.Type {
		case NodeText:
			if node.Text != "" {
				runs = append(runs, textRun{
					text:     node.Text,
					language: nodeLanguage,
					voice:    nodeVoice,
				})
			}
		case NodeUnknown:
			if !opts.OmitUnknown && node.Raw != "" {
				runs = append(runs, textRun{
					text:     node.Raw,
					language: nodeLanguage,
					voice:    nodeVoice,
				})
			}
		case NodeTimestamp:
			if opts.KeepTimestamps && node.StartTag != "" {
				runs = append(runs, textRun{
					text:     node.StartTag,
					language: nodeLanguage,
					voice:    nodeVoice,
				})
			}
		default:
			runs = append(runs, collectRuns(node.Children, nodeLanguage, nodeVoice, opts)...)
		}
	}
	return runs
}

func joinRuns(runs []textRun, preferred string, bestScore int) string {
	var out strings.Builder
	for _, run := range runs {
		if bestScore == 0 || run.language == "" ||
			languageMatchScore(preferred, run.language) == bestScore {
			out.WriteString(run.text)
		}
	}
	return out.String()
}

func firstLanguage(runs []textRun) string {
	for _, run := range runs {
		if run.language != "" {
			return run.language
		}
	}
	return ""
}

func runVoices(runs []textRun) []string {
	seen := make(map[string]struct{})
	var voices []string
	for _, run := range runs {
		if run.voice == "" {
			continue
		}
		if _, ok := seen[run.voice]; ok {
			continue
		}
		seen[run.voice] = struct{}{}
		voices = append(voices, run.voice)
	}
	return voices
}

// NormalizeLanguage canonicalizes a BCP 47 language tag for comparison.
func NormalizeLanguage(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.ReplaceAll(tag, "_", "-")
	return strings.ToLower(tag)
}

// LanguageMatch reports whether actual can satisfy preferred under BCP 47
// prefix fallback. An empty actual tag only matches an empty preference.
func LanguageMatch(preferred, actual string) bool {
	preferred = NormalizeLanguage(preferred)
	actual = NormalizeLanguage(actual)
	if preferred == "" {
		return actual == ""
	}
	return languageMatchScore(preferred, actual) > 0
}

func languageMatchScore(preferred, actual string) int {
	preferred = NormalizeLanguage(preferred)
	actual = NormalizeLanguage(actual)
	if preferred == "" || actual == "" {
		return 0
	}
	if preferred == actual {
		return 10000 + strings.Count(actual, "-")
	}
	if strings.HasPrefix(actual, preferred+"-") {
		return 500 + strings.Count(preferred, "-")
	}

	parts := strings.Split(preferred, "-")
	for i := len(parts) - 1; i >= 1; i-- {
		prefix := strings.Join(parts[:i], "-")
		if actual == prefix {
			return 1000 + i
		}
	}
	return 0
}

// PreferredLanguage returns the best matching language tag from candidates.
// Untagged candidates are only returned when preferred is empty.
func PreferredLanguage(preferred string, candidates []string) (string, bool) {
	if NormalizeLanguage(preferred) == "" {
		for _, candidate := range candidates {
			if NormalizeLanguage(candidate) == "" {
				return candidate, true
			}
		}
		return "", false
	}
	type scored struct {
		value string
		score int
	}
	var matches []scored
	for _, candidate := range candidates {
		if score := languageMatchScore(preferred, candidate); score > 0 {
			matches = append(matches, scored{value: candidate, score: score})
		}
	}
	if len(matches) == 0 {
		return "", false
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})
	return matches[0].value, true
}

package plan

import (
	"fmt"
	"sort"
	"strings"

	"srtplan/internal/srt"
)

func buildCoverage(cues []*Cue, items []Item) []Coverage {
	byCue := map[string][]Item{}
	for _, item := range items {
		byCue[item.CueID] = append(byCue[item.CueID], item)
	}

	var out []Coverage
	for _, cue := range cues {
		if strings.TrimSpace(cue.DisplayText) == "" {
			continue
		}
		cov := Coverage{
			CueID:      cue.ID,
			Source:     cue.Source,
			SourceSeq:  cue.SourceSeq,
			OriginalMs: cue.EndMs - cue.StartMs,
		}
		for _, item := range byCue[cue.ID] {
			duration := item.EndMs - item.StartMs
			if item.Deferred {
				cov.DeferredMs += duration
			} else {
				cov.PlacedMs += duration
			}
		}
		cov.Segments = len(byCue[cue.ID])
		cov.Complete = cov.PlacedMs+cov.DeferredMs >= cov.OriginalMs
		out = append(out, cov)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CueID < out[j].CueID })
	return out
}

func buildStats(cues []*Cue, items []Item) Stats {
	content := 0
	for _, cue := range cues {
		if strings.TrimSpace(cue.DisplayText) != "" {
			content++
		}
	}
	stats := Stats{
		InputCues:   len(cues),
		ContentCues: content,
		OutputItems: len(items),
	}
	splitCues, manualCues := map[string]bool{}, map[string]bool{}
	for _, item := range items {
		if item.Deferred {
			stats.DeferredItems++
		} else {
			stats.PlacedItems++
		}
		if item.Split {
			splitCues[item.CueID] = true
		}
		if item.Manual {
			manualCues[item.CueID] = true
		}
	}
	stats.SplitCues = len(splitCues)
	stats.ManualCues = len(manualCues)
	return stats
}

// verifyNoContentDropped is the hard guarantee behind the tool: every cue that
// carries text must have enough scheduled time to cover its original window.
func verifyNoContentDropped(cues []*Cue, coverage []Coverage) error {
	index := map[string]Coverage{}
	for _, cov := range coverage {
		index[cov.CueID] = cov
	}
	var missing []string
	for _, cue := range cues {
		if strings.TrimSpace(cue.DisplayText) == "" {
			continue
		}
		cov, ok := index[cue.ID]
		if !ok || cov.PlacedMs+cov.DeferredMs < cov.OriginalMs {
			missing = append(missing, cue.ID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("plan: internal error, content lost for cues: %s",
			strings.Join(missing, ", "))
	}
	return nil
}

// RenderSRT converts report items into renderable SubRip cues.
func RenderSRT(items []Item) []srt.RenderCue {
	out := make([]srt.RenderCue, 0, len(items))
	for _, item := range items {
		region := srt.RegionBottom
		if item.Region == RegionTop {
			region = srt.RegionTop
		}
		out = append(out, srt.RenderCue{
			Start:  item.StartMs,
			End:    item.EndMs,
			Region: region,
			Lines:  item.Lines,
		})
	}
	return out
}

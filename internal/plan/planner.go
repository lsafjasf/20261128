package plan

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type busyInterval struct {
	start, end int64
}

type blockedFragment struct {
	cue         *Cue
	windowStart int64
	windowEnd   int64
}

// Plan runs the full engine and returns the explainable report.
func Plan(inputs []Input, opts Options) (*Report, error) {
	cues, sources, err := Build(inputs, opts)
	if err != nil {
		return nil, err
	}
	windows, _ := sweep(cues, inputs)

	items := buildPlacedItems(windows)
	blocked := collectBlocked(windows)
	items = append(items, scheduleDeferred(blocked, items, opts)...)

	items = finalizeSegments(items)
	coverage := buildCoverage(cues, items)
	stats := buildStats(cues, items)

	if err := verifyNoContentDropped(cues, coverage); err != nil {
		return nil, err
	}

	return &Report{
		Sources:     sources,
		Items:       items,
		Coverage:    coverage,
		Stats:       stats,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func buildPlacedItems(windows []windowResult) []Item {
	var items []Item
	for _, w := range windows {
		owners := make([]*Cue, 0, len(w.slots))
		for _, cue := range w.slots {
			owners = append(owners, cue)
		}
		sort.Slice(owners, func(i, j int) bool { return owners[i].ID < owners[j].ID })

		for _, cue := range owners {
			region := regionOf(w.slots, cue)
			items = append(items, Item{
				CueID:           cue.ID,
				Source:          cue.Source,
				SourceSeq:       cue.SourceSeq,
				Region:          region,
				StartMs:         w.startMs,
				EndMs:           w.endMs,
				StartTC:         tc(w.startMs),
				EndTC:           tc(w.endMs),
				OriginalStartMs: w.startMs,
				OriginalEndMs:   w.endMs,
				Deferred:        false,
				Language:        cue.Language,
				Speaker:         cue.Speaker,
				Manual:          cue.Manual,
				Reasons:         placedReasons(cue, w, region),
				Lines:           renderLines(cue),
			})
		}
	}
	return items
}

func regionOf(slots map[string]*Cue, cue *Cue) string {
	for region, owner := range slots {
		if owner == cue {
			return region
		}
	}
	return ""
}

func placedReasons(cue *Cue, w windowResult, region string) []string {
	var reasons []string
	if cue.Manual {
		reasons = append(reasons, fmt.Sprintf(
			"manual marker overrides %d competing automatic cue(s)",
			automaticCompetitors(cue, w.active)))
	}
	if cue.PreferredRegion != "" {
		reasons = append(reasons, "explicit region preference honored: "+cue.PreferredRegion)
	} else if cue.Positioned {
		reasons = append(reasons, "coordinates on timestamp line map to region: "+region)
	} else if cue.SourceIndex == 0 {
		reasons = append(reasons, "primary language track keeps the bottom region")
	} else {
		reasons = append(reasons, "secondary/translation track uses the top region")
	}
	if cue.Speaker != "" {
		reasons = append(reasons, "speaker label '"+cue.Speaker+"' kept as structured metadata, not concatenated")
	}
	if cue.Language != "" && cue.Language != "und" {
		reasons = append(reasons, "language detected as "+cue.Language+"; different languages never share one line")
	}
	return reasons
}

func automaticCompetitors(self *Cue, active []*Cue) int {
	n := 0
	for _, cue := range active {
		if cue != self && !cue.Manual {
			n++
		}
	}
	return n
}

func renderLines(cue *Cue) []string {
	textLines := strings.Split(cue.DisplayText, "\n")
	if cue.Speaker == "" || len(textLines) == 0 {
		return textLines
	}
	textLines[0] = cue.Speaker + ": " + textLines[0]
	return textLines
}

func collectBlocked(windows []windowResult) []blockedFragment {
	var blocked []blockedFragment
	for _, w := range windows {
		for _, cue := range w.blocked {
			blocked = append(blocked, blockedFragment{
				cue:         cue,
				windowStart: w.startMs,
				windowEnd:   w.endMs,
			})
		}
	}
	sort.Slice(blocked, func(i, j int) bool {
		if blocked[i].windowStart != blocked[j].windowStart {
			return blocked[i].windowStart < blocked[j].windowStart
		}
		return blocked[i].cue.ID < blocked[j].cue.ID
	})
	return blocked
}

// scheduleDeferred pushes every blocked fragment to the earliest free slot
// after its original window. The fragment keeps its full cue text; only the
// timing changes, and the report records both original and effective times.
func scheduleDeferred(blocked []blockedFragment, placed []Item, opts Options) []Item {
	busy := map[string][]busyInterval{}
	for _, item := range placed {
		busy[item.Region] = append(busy[item.Region], busyInterval{item.StartMs, item.EndMs})
	}

	var deferred []Item
	for _, fragment := range blocked {
		cue := fragment.cue
		duration := fragment.windowEnd - fragment.windowStart
		region, start := earliestFree(cue, fragment.windowEnd, duration, busy)
		end := start + duration
		busy[region] = append(busy[region], busyInterval{start, end})

		reasons := []string{
			fmt.Sprintf("both regions occupied at %s; deferred to %s keeping full text",
				tc(fragment.windowStart), tc(start)),
			"deferred items never replace placed content and never delete the cue",
		}
		if opts.MaxDeferralMs > 0 && start-fragment.windowStart > opts.MaxDeferralMs {
			reasons = append(reasons, fmt.Sprintf(
				"deferral of %dms exceeds configured cap %dms; still scheduled so no content is lost",
				start-fragment.windowStart, opts.MaxDeferralMs))
		}
		deferred = append(deferred, Item{
			CueID:           cue.ID,
			Source:          cue.Source,
			SourceSeq:       cue.SourceSeq,
			Region:          region,
			StartMs:         start,
			EndMs:           end,
			StartTC:         tc(start),
			EndTC:           tc(end),
			OriginalStartMs: fragment.windowStart,
			OriginalEndMs:   fragment.windowEnd,
			Deferred:        true,
			Language:        cue.Language,
			Speaker:         cue.Speaker,
			Manual:          cue.Manual,
			Reasons:         reasons,
			Lines:           renderLines(cue),
		})
	}
	return deferred
}

func earliestFree(cue *Cue, notBefore, duration int64, busy map[string][]busyInterval) (string, int64) {
	bestRegion, bestStart := "", int64(-1)
	for _, region := range regionOrder(cue) {
		start := firstFreeAfter(notBefore, duration, busy[region])
		if bestStart == -1 || start < bestStart {
			bestRegion, bestStart = region, start
		}
	}
	return bestRegion, bestStart
}

func firstFreeAfter(notBefore, duration int64, intervals []busyInterval) int64 {
	sorted := append([]busyInterval(nil), intervals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].start < sorted[j].start })
	candidate := notBefore
	for {
		collision := false
		for _, iv := range sorted {
			if iv.end <= candidate {
				continue
			}
			if iv.start >= candidate+duration {
				continue
			}
			candidate = iv.end
			collision = true
		}
		if !collision {
			return candidate
		}
	}
}

// finalizeSegments merges adjacent same-cue/same-region items back into one
// item where possible and assigns 1-based segment numbers in time order.
func finalizeSegments(items []Item) []Item {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CueID != items[j].CueID {
			return items[i].CueID < items[j].CueID
		}
		return items[i].StartMs < items[j].StartMs
	})

	var merged []Item
	for _, item := range items {
		item.Split = false
		if len(merged) > 0 {
			prev := &merged[len(merged)-1]
			if canCoalesce(*prev, item) {
				prev.EndMs = item.EndMs
				prev.EndTC = item.EndTC
				prev.OriginalEndMs = item.OriginalEndMs
				continue
			}
		}
		merged = append(merged, item)
	}

	counts := map[string]int{}
	for i := range merged {
		counts[merged[i].CueID]++
	}
	splitCues := map[string]bool{}
	segmentOf := map[string]int{}
	for i := range merged {
		cueID := merged[i].CueID
		segmentOf[cueID]++
		merged[i].Segment = segmentOf[cueID]
		if counts[cueID] > 1 {
			merged[i].Split = true
			splitCues[cueID] = true
		}
		merged[i].Decision = decide(merged[i])
	}

	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].StartMs != merged[j].StartMs {
			return merged[i].StartMs < merged[j].StartMs
		}
		if merged[i].Region != merged[j].Region {
			return merged[i].Region < merged[j].Region
		}
		return merged[i].CueID < merged[j].CueID
	})
	return merged
}

func canCoalesce(prev, next Item) bool {
	if prev.CueID != next.CueID || prev.Region != next.Region {
		return false
	}
	if prev.Deferred != next.Deferred || prev.EndMs != next.StartMs {
		return false
	}
	return true
}

func decide(item Item) string {
	switch {
	case item.Deferred && item.Split:
		return DecisionSplitDeferred
	case item.Deferred && item.Manual:
		return DecisionManualDeferred
	case item.Deferred:
		return DecisionDeferred
	case item.Manual:
		return DecisionManualPlaced
	case item.Split:
		return DecisionSplitPlaced
	default:
		return DecisionPlaced
	}
}

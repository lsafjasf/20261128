package plan

import (
	"sort"
	"strings"
)

type windowResult struct {
	startMs, endMs int64
	active         []*Cue
	slots          map[string]*Cue
	blocked        []*Cue
}

// sweep cuts the timeline at every cue boundary and assigns the two vertical
// regions inside each window. Cues that lose both regions are recorded as
// blocked and scheduled later via the deferral pass.
func sweep(cues []*Cue, inputs []Input) ([]windowResult, map[string]*Cue) {
	bounds := boundarySet(cues)
	index := indexByID(cues)

	var windows []windowResult
	for i := 0; i+1 < len(bounds); i++ {
		start, end := bounds[i], bounds[i+1]
		active := activeCues(cues, start, end)
		if len(active) == 0 {
			continue
		}
		sort.SliceStable(active, func(a, b int) bool {
			return higherPriority(active[a], active[b], inputs)
		})

		slots := map[string]*Cue{}
		var blocked []*Cue
		for _, cue := range active {
			region, ok := claimRegion(cue, slots)
			if ok {
				slots[region] = cue
				continue
			}
			blocked = append(blocked, cue)
		}
		windows = append(windows, windowResult{
			startMs: start,
			endMs:   end,
			active:  active,
			slots:   slots,
			blocked: blocked,
		})
	}
	return windows, index
}

func boundarySet(cues []*Cue) []int64 {
	set := map[int64]struct{}{}
	for _, cue := range cues {
		if strings.TrimSpace(cue.DisplayText) == "" {
			continue
		}
		set[cue.StartMs] = struct{}{}
		set[cue.EndMs] = struct{}{}
	}
	out := make([]int64, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func indexByID(cues []*Cue) map[string]*Cue {
	index := map[string]*Cue{}
	for _, cue := range cues {
		index[cue.ID] = cue
	}
	return index
}

func activeCues(cues []*Cue, start, end int64) []*Cue {
	var active []*Cue
	for _, cue := range cues {
		if strings.TrimSpace(cue.DisplayText) == "" {
			continue
		}
		if cue.StartMs <= start && cue.EndMs >= end {
			active = append(active, cue)
		}
	}
	return active
}

// higherPriority defines the global ordering applied within every window:
// manual cues first (their numeric marker wins ties), then explicit region
// preferences, then primary-language tracks, then source order, then the
// earlier original start time, and finally the cue id for determinism.
func higherPriority(a, b *Cue, inputs []Input) bool {
	am, bm := manualRank(a), manualRank(b)
	if am != bm {
		return am > bm
	}
	ae, be := explicitRank(a), explicitRank(b)
	if ae != be {
		return ae > be
	}
	ap, bp := primaryRank(a, inputs), primaryRank(b, inputs)
	if ap != bp {
		return ap > bp
	}
	if a.SourceIndex != b.SourceIndex {
		return a.SourceIndex < b.SourceIndex
	}
	if a.StartMs != b.StartMs {
		return a.StartMs < b.StartMs
	}
	return a.ID < b.ID
}

func manualRank(c *Cue) int {
	if !c.Manual {
		return 0
	}
	// Manual beats automatic; an explicit numeric marker breaks ties between
	// two manual cues. Base 100 keeps any marker above the automatic floor.
	return 100 + c.ManualPriority
}

func explicitRank(c *Cue) int {
	if c.PreferredRegion != "" || c.Positioned {
		return 1
	}
	return 0
}

func primaryRank(c *Cue, inputs []Input) int {
	if c.SourceIndex >= 0 && c.SourceIndex < len(inputs) && inputs[c.SourceIndex].Primary {
		return 1
	}
	return 0
}

// claimRegion honours an explicit preference first; otherwise the first
// (primary) source takes bottom and secondary/translation sources take top.
func claimRegion(c *Cue, slots map[string]*Cue) (string, bool) {
	for _, region := range regionOrder(c) {
		if _, taken := slots[region]; !taken {
			return region, true
		}
	}
	return "", false
}

func regionOrder(c *Cue) []string {
	if c.PreferredRegion != "" {
		if c.PreferredRegion == RegionTop {
			return []string{RegionTop, RegionBottom}
		}
		return []string{RegionBottom, RegionTop}
	}
	if c.SourceIndex == 0 {
		return []string{RegionBottom, RegionTop}
	}
	return []string{RegionTop, RegionBottom}
}

package webvtt

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// MergeOptions controls cue merging.
//
// By default only cues with overlapping intervals are merged. MergeAdjacent
// also merges same-speaker cues separated by at most MaxGap.
type MergeOptions struct {
	MergeAdjacent bool
	MaxGap        time.Duration
}

// MergeOverlapping merges overlapping cues only when their complete speaker
// sets are equal. Untagged cues are considered to have an empty speaker set and
// therefore are not merged with voiced cues.
func MergeOverlapping(cues []Cue) []Cue {
	return MergeCues(cues, MergeOptions{})
}

// MergeOverlappingCues is an alias for MergeOverlapping.
func MergeOverlappingCues(cues []Cue) []Cue {
	return MergeOverlapping(cues)
}

// MergeCues returns cues in timeline order, merging eligible neighbours.
// The input slice and its cue values are not modified.
func MergeCues(cues []Cue, opts MergeOptions) []Cue {
	ordered := make([]Cue, len(cues))
	for i := range cues {
		ordered[i] = parsedCue(cues[i])
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Start != ordered[j].Start {
			return ordered[i].Start < ordered[j].Start
		}
		return ordered[i].End < ordered[j].End
	})

	var merged []Cue
	groupIndex := make(map[string]int)
	for _, cue := range ordered {
		key := CueSpeakerKey(cue)
		group, ok := groupIndex[key]
		if !ok {
			groupIndex[key] = len(merged)
			merged = append(merged, cue)
			continue
		}
		last := &merged[group]
		if canMerge(*last, cue, opts) {
			*last = mergeCuePair(*last, cue)
		} else {
			groupIndex[key] = len(merged)
			merged = append(merged, cue)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Start != merged[j].Start {
			return merged[i].Start < merged[j].Start
		}
		return merged[i].End < merged[j].End
	})
	return merged
}

// Speakers returns the voice annotations in c in payload order, without
// duplicates.
func (c Cue) Speakers() []string {
	c = parsedCue(c)
	var speakers []string
	seen := make(map[string]struct{})
	var visit func([]Node)
	visit = func(nodes []Node) {
		for _, node := range nodes {
			if node.Type == NodeVoice && node.Voice != "" {
				if _, ok := seen[node.Voice]; !ok {
					seen[node.Voice] = struct{}{}
					speakers = append(speakers, node.Voice)
				}
			}
			visit(node.Children)
		}
	}
	visit(c.Nodes)
	return speakers
}

// Speaker returns the first voice annotation in c, or an empty string when the
// cue has no voice tag.
func (c Cue) Speaker() string {
	speakers := c.Speakers()
	if len(speakers) == 0 {
		return ""
	}
	return speakers[0]
}

func parsedCue(c Cue) Cue {
	if c.Nodes == nil && c.Payload != "" {
		c.Nodes = ParseCueMarkup(c.Payload)
	}
	return c
}

func canMerge(a, b Cue, opts MergeOptions) bool {
	if !sameSpeakers(a.Speakers(), b.Speakers()) {
		return false
	}
	if b.Start < a.End {
		return true
	}
	if opts.MergeAdjacent {
		gap := b.Start - a.End
		return gap >= 0 && gap <= opts.MaxGap
	}
	return false
}

func sameSpeakers(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mergeCuePair(a, b Cue) Cue {
	if b.End > a.End {
		a.End = b.End
	}
	if a.ID == "" {
		a.ID = b.ID
	}
	if a.Settings == "" {
		a.Settings = b.Settings
	}
	switch {
	case a.Payload == "":
		a.Payload = b.Payload
	case b.Payload == "":
	default:
		a.Payload += "\n" + b.Payload
	}
	if a.Payload == "" {
		a.Nodes = nil
	} else {
		a.Nodes = ParseCueMarkup(a.Payload)
	}
	return a
}

// CueSpeakerKey returns a stable comparison key for a cue's speaker set.
func CueSpeakerKey(c Cue) string {
	speakers := append([]string(nil), c.Speakers()...)
	sort.Strings(speakers)
	var key strings.Builder
	for _, speaker := range speakers {
		key.WriteString(strconv.Itoa(len(speaker)))
		key.WriteByte(':')
		key.WriteString(speaker)
		key.WriteByte(';')
	}
	return key.String()
}

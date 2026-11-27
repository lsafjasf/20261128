// Package plan turns multiple overlapping SRT inputs into an explainable
// on-screen layout. Cues are never dropped: every cue with text is either
// placed in a vertical region during its original time window, split across
// windows when another cue takes over the region, or deferred to the first
// free slot and marked as such in the report.
package plan

import (
	"time"

	"srtplan/internal/srt"
)

// Region identifiers shared by the engine and its report.
const (
	RegionTop    = "top"
	RegionBottom = "bottom"
)

// Decision codes explain how each produced item was handled.
const (
	DecisionPlaced         = "placed"
	DecisionSplitPlaced    = "split_placed"
	DecisionDeferred       = "deferred"
	DecisionSplitDeferred  = "split_deferred"
	DecisionManualPlaced   = "manual_placed"
	DecisionManualDeferred = "manual_deferred"
)

// Input is one parsed SRT file participating in the merge.
type Input struct {
	Name     string
	Priority int  // higher wins; manual markers inside cues still beat this
	Primary  bool // primary-language track; defaults to bottom region
	File     *srt.File
}

// Options controls engine behaviour.
type Options struct {
	// CanvasY is the nominal video height used to classify Y1 coordinates.
	CanvasY int
	// MaxDeferralMs bounds how far past its original window a cue may be
	// pushed. Zero means no bound.
	MaxDeferralMs int64
}

// Cue is the engine's enriched view of one parsed SRT cue.
type Cue struct {
	ID              string
	Source          string
	SourceIndex     int
	SourceSeq       int
	StartMs         int64
	EndMs           int64
	RawText         string
	DisplayText     string
	Speaker         string
	Language        string
	Manual          bool
	ManualPriority  int
	PreferredRegion string
	Positioned      bool
}

// Item is one scheduled on-screen fragment of a cue.
type Item struct {
	CueID           string   `json:"cueId"`
	Source          string   `json:"source"`
	SourceSeq       int      `json:"sourceSeq"`
	Segment         int      `json:"segment"`
	Region          string   `json:"region"`
	StartMs         int64    `json:"startMs"`
	EndMs           int64    `json:"endMs"`
	StartTC         string   `json:"start"`
	EndTC           string   `json:"end"`
	OriginalStartMs int64    `json:"originalStartMs"`
	OriginalEndMs   int64    `json:"originalEndMs"`
	Deferred        bool     `json:"deferred"`
	Language        string   `json:"language"`
	Speaker         string   `json:"speaker,omitempty"`
	Manual          bool     `json:"manual"`
	Split           bool     `json:"split"`
	Decision        string   `json:"decision"`
	Reasons         []string `json:"reasons"`
	Lines           []string `json:"-"`
}

// Coverage tracks how much of each cue made it onto the timeline.
type Coverage struct {
	CueID      string `json:"cueId"`
	Source     string `json:"source"`
	SourceSeq  int    `json:"sourceSeq"`
	OriginalMs int64  `json:"originalMs"`
	PlacedMs   int64  `json:"placedMs"`
	DeferredMs int64  `json:"deferredMs"`
	Segments   int    `json:"segments"`
	Complete   bool   `json:"complete"`
}

// Report is the full explainable plan.
type Report struct {
	Sources     []SourceInfo `json:"sources"`
	Items       []Item       `json:"items"`
	Coverage    []Coverage   `json:"coverage"`
	Stats       Stats        `json:"stats"`
	GeneratedAt string       `json:"generatedAt"`
}

// SourceInfo describes one input track.
type SourceInfo struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
	Primary  bool   `json:"primary"`
	CueCount int    `json:"cueCount"`
}

// Stats summarises engine decisions.
type Stats struct {
	InputCues     int `json:"inputCues"`
	ContentCues   int `json:"contentCues"`
	OutputItems   int `json:"outputItems"`
	PlacedItems   int `json:"placedItems"`
	DeferredItems int `json:"deferredItems"`
	SplitCues     int `json:"splitCues"`
	ManualCues    int `json:"manualCues"`
	DroppedCues   int `json:"droppedCues"`
}

func ms(d time.Duration) int64 {
	return d.Milliseconds()
}

func tc(v int64) string {
	return srt.FormatTime(time.Duration(v) * time.Millisecond)
}

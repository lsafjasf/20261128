package main

import (
	"fmt"
	"strings"

	"imscfit"
)

type fullReport struct {
	Source     string                          `json:"source"`
	Canvases   []imscfit.CanvasReport          `json:"canvases"`
	Candidates map[string][]imscfit.Candidates `json:"candidates,omitempty"`
}

func printText(rep fullReport) {
	fmt.Printf("IMSC layout report: %s\n\n", rep.Source)
	for _, cv := range rep.Canvases {
		fmt.Printf("== Canvas %s (%.0fx%.0f) ==\n", cv.Canvas.Name, cv.Canvas.Width, cv.Canvas.Height)
		for _, rr := range cv.Regions {
			g := rr.Geometry
			fmt.Printf("region %-12s origin=(%.0f,%.0f) extent=%.0fx%.0f padding=T%.0f R%.0f B%.0f L%.0f content=%.0fx%.0f",
				rr.ID, g.OriginX, g.OriginY, g.Width, g.Height,
				g.PadTop, g.PadRight, g.PadBottom, g.PadLeft, g.ContentW, g.ContentH)
			fmt.Printf(" displayAlign=%s overflow=%s wrap=%s",
				orDefault(rr.Props.DisplayAlign, "before"),
				orDefault(rr.Props.Overflow, "visible"),
				orDefault(rr.Props.WrapOption, "wrap"))
			if g.OffCanvas {
				fmt.Print(" [OFF-CANVAS]")
			}
			fmt.Println()
		}
		for _, cue := range cv.Cues {
			printCue(cue)
		}
		for _, cand := range rep.Candidates[cv.Canvas.Name] {
			printCandidates(cand)
		}
		fmt.Println()
	}
}

func printCue(cue imscfit.CueReport) {
	label := cue.ParagraphID
	if label == "" {
		label = cue.Begin
	}
	fmt.Printf("  cue %-10s [%s] region=%s lines=%d used=%.0fx%.0f budget=%.0fx%.0f",
		label, cue.Status, orDefault(cue.RegionID, "-"),
		cue.LineCount, cue.UsedWidth, cue.UsedHeight, cue.ContentW, cue.ContentH)
	if cue.OverflowX > 0.5 || cue.OverflowY > 0.5 {
		fmt.Printf(" overflow=%.0fx%.0f", cue.OverflowX, cue.OverflowY)
	}
	if cue.SpillTop > 0.5 || cue.SpillBottom > 0.5 {
		fmt.Printf(" spill(top=%.0f,bottom=%.0f)", cue.SpillTop, cue.SpillBottom)
	}
	if cue.ClippedLines > 0 {
		fmt.Printf(" clippedLines=%d", cue.ClippedLines)
	}
	fmt.Println()
	for i, ln := range cue.Lines {
		fmt.Printf("      L%d: %q (%.0fpx)\n", i+1, ln.Text, ln.Width)
	}
}

func printCandidates(cand imscfit.Candidates) {
	fmt.Printf("  candidates for %s:\n", cand.ParagraphID)
	for _, t := range cand.ShortenText {
		fmt.Printf("    - shorten: %q lines=%d used=%.0fx%.0f removed=%d runes\n",
			t.Text, t.LineCount, t.UsedWidth, t.UsedHeight, t.RemovedRunes)
	}
	for _, r := range cand.EnlargeRegion {
		fmt.Printf("    - region:  origin=%s extent=%s (%.0fx%.0f) fits=%t",
			r.Origin, r.Extent, r.Width, r.Height, r.FitsCanvas)
		if strings.TrimSpace(r.Note) != "" {
			fmt.Printf(" note=%q", r.Note)
		}
		fmt.Println()
	}
}

func orDefault(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

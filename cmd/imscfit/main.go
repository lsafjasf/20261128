package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"imscfit"
)

func main() {
	var (
		canvasFlag = flag.String("canvas", "",
			"comma-separated name=WxH list; defaults to common broadcast/mobile aspect ratios")
		jsonOut = flag.Bool("json", false, "emit a machine-readable JSON report")
	)
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: imscfit [--canvas name=WxH,...] [--json] file.ttml")
		os.Exit(2)
	}

	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fatal(err)
	}
	defer f.Close()

	doc, err := imscfit.Parse(f)
	if err != nil {
		fatal(err)
	}

	canvases, err := resolveCanvases(*canvasFlag, doc)
	if err != nil {
		fatal(err)
	}

	rep := fullReport{Source: flag.Arg(0), Candidates: map[string][]imscfit.Candidates{}}

	for _, cv := range canvases {
		cr := doc.Analyze(cv)
		rep.Canvases = append(rep.Canvases, cr)
		for _, cue := range cr.Cues {
			if cue.Status == imscfit.FitOK || cue.Status == imscfit.FitNoRegion {
				continue
			}
			p := findParagraph(doc, cue.ParagraphID)
			if p == nil {
				continue
			}
			rep.Candidates[cv.Name] = append(rep.Candidates[cv.Name], doc.BuildCandidates(p, cv))
		}
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fatal(err)
		}
		return
	}
	printText(rep)
}

func resolveCanvases(flagValue string, doc *imscfit.Document) ([]imscfit.Canvas, error) {
	if strings.TrimSpace(flagValue) != "" {
		var out []imscfit.Canvas
		for _, item := range strings.Split(flagValue, ",") {
			parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("bad --canvas entry %q", item)
			}
			wh := strings.SplitN(parts[1], "x", 2)
			if len(wh) != 2 {
				return nil, fmt.Errorf("bad --canvas size %q", parts[1])
			}
			w, err1 := strconv.ParseFloat(strings.TrimSpace(wh[0]), 64)
			h, err2 := strconv.ParseFloat(strings.TrimSpace(wh[1]), 64)
			if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
				return nil, fmt.Errorf("bad --canvas size %q", parts[1])
			}
			out = append(out, imscfit.Canvas{Name: strings.TrimSpace(parts[0]), Width: w, Height: h})
		}
		return out, nil
	}

	height := 1080.0
	if doc.RootExtent.Y.Set && doc.RootExtent.Y.Unit == imscfit.UnitPX {
		height = doc.RootExtent.Y.Value
	}
	return []imscfit.Canvas{
		{Name: "16:9", Width: height * 16 / 9, Height: height},
		{Name: "4:3", Width: height * 4 / 3, Height: height},
		{Name: "2.39:1", Width: height * 2.39, Height: height},
		{Name: "9:16", Width: height * 9 / 16, Height: height},
	}, nil
}

func findParagraph(doc *imscfit.Document, id string) *imscfit.Paragraph {
	for _, p := range doc.Paragraphs {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "imscfit:", err)
	os.Exit(1)
}

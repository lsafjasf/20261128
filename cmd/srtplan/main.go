// Command srtplan merges overlapping SRT tracks into an explainable display
// plan. It never concatenates bilingual lines or speaker labels, never drops
// a cue that contains text, and records why every fragment was placed, split
// or deferred.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"srtplan/internal/plan"
	"srtplan/internal/srt"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "srtplan:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("srtplan", flag.ContinueOnError)
	primary := fs.String("primary", "", "path to the primary-language SRT (rendered at the bottom)")
	secondary := fs.String("secondary", "", "path to the translation/second SRT (rendered at the top)")
	secondaryName := fs.String("secondary-name", "secondary", "logical name of the secondary track")
	reportPath := fs.String("report", "plan.json", "output path for the explainable JSON report")
	outPath := fs.String("out", "merged.srt", "output path for the rendered SRT (empty disables SRT output)")
	maxDefer := fs.Int64("max-deferral-ms", 0, "warn when a deferred cue moves later than this many milliseconds")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *primary == "" || *secondary == "" {
		return fmt.Errorf("both -primary and -secondary SRT paths are required")
	}

	primaryFile, err := readSRT(*primary)
	if err != nil {
		return err
	}
	secondaryFile, err := readSRT(*secondary)
	if err != nil {
		return err
	}

	report, err := plan.Plan([]plan.Input{
		{Name: "primary", Priority: 10, Primary: true, File: primaryFile},
		{Name: *secondaryName, Priority: 5, File: secondaryFile},
	}, plan.Options{MaxDeferralMs: *maxDefer})
	if err != nil {
		return err
	}

	if err := writeJSON(*reportPath, report); err != nil {
		return err
	}
	if *outPath != "" {
		rendered := srt.Render(plan.RenderSRT(report.Items))
		if err := os.WriteFile(*outPath, []byte(rendered), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", *outPath, err)
		}
	}

	fmt.Printf("srtplan: %d content cues -> %d items (%d placed, %d deferred, %d split)\n",
		report.Stats.ContentCues, report.Stats.OutputItems,
		report.Stats.PlacedItems, report.Stats.DeferredItems, report.Stats.SplitCues)
	fmt.Printf("srtplan: report %s", *reportPath)
	if *outPath != "" {
		fmt.Printf(", srt %s", *outPath)
	}
	fmt.Println()
	return nil
}

func readSRT(path string) (*srt.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	parsed, err := srt.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	parsed.Path = path
	return parsed, nil
}

func writeJSON(path string, report *plan.Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

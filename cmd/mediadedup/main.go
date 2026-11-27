// mediadedup builds rotation/fps/audio-aware deduplication candidate groups
// over a media pool and performs group-scoped master replacement on timelines.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"mediadedup/dedup"
	"mediadedup/media"
	"mediadedup/timeline"
)

type inputFile struct {
	Assets []media.Asset     `json:"assets"`
	Clips  []timeline.Clip   `json:"clips"`
	Master map[string]string `json:"masters"` // groupID -> assetID
}

type groupJSON struct {
	ID       string             `json:"id"`
	MasterID string             `json:"masterId"`
	Members  []string           `json:"members"`
	Pairs    []dedup.PairResult `json:"pairs"`
}

type outputFile struct {
	Groups     []groupJSON        `json:"groups"`
	Singletons []string           `json:"singletons"`
	Reports    []*timeline.Report `json:"replacementReports"`
	Clips      []timeline.Clip    `json:"clips"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mediadedup:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("mediadedup", flag.ContinueOnError)
	demo := fs.Bool("demo", false, "run the built-in end-to-end scenario")
	in := fs.String("in", "", "path to JSON input (assets, clips, masters)")
	outPath := fs.String("out", "", "write JSON result to this path instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *demo {
		return runDemo(os.Stdout)
	}
	if *in == "" {
		fs.Usage()
		return fmt.Errorf("either -demo or -in <file.json> is required")
	}
	return runFile(*in, *outPath)
}

func runFile(inPath, outPath string) error {
	raw, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	var in inputFile
	if err := json.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("parsing %s: %w", inPath, err)
	}

	pool := dedup.Build(in.Assets, dedup.DefaultThresholds())

	result := outputFile{Clips: in.Clips}
	for _, g := range pool.Groups {
		if id := resolveMaster(in.Master, g); id != "" {
			if err := g.SetMaster(id); err != nil {
				return err
			}
		}
		gj := groupJSON{ID: g.ID, MasterID: g.MasterID}
		for _, m := range g.Members {
			gj.Members = append(gj.Members, m.ID)
		}
		for _, p := range g.Relations {
			gj.Pairs = append(gj.Pairs, p)
		}
		result.Groups = append(result.Groups, gj)
	}
	for _, a := range in.Assets {
		if _, ok := pool.GroupOf(a.ID); !ok {
			result.Singletons = append(result.Singletons, a.ID)
		}
	}

	tl := &timeline.Timeline{Clips: in.Clips}
	for _, g := range pool.Groups {
		report, err := timeline.ReplaceWithMaster(tl, pool, g.ID)
		if err != nil {
			return err
		}
		result.Reports = append(result.Reports, report)
	}
	result.Clips = tl.Clips

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if outPath == "" {
		fmt.Println(string(encoded))
		return nil
	}
	return os.WriteFile(outPath, append(encoded, '\n'), 0o644)
}

// resolveMaster accepts either a group ID key or any member asset ID key.
func resolveMaster(masters map[string]string, g *dedup.Group) string {
	if id, ok := masters[g.ID]; ok {
		return id
	}
	for key, id := range masters {
		if g.HasMember(key) {
			return id
		}
	}
	return ""
}

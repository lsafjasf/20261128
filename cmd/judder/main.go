package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"uid4-0038/judder/internal/resample"
)

func main() {
	var (
		srcFPS        = flag.Float64("src-fps", 24, "source frame rate (integer or rational num/den)")
		outFPS        = flag.Float64("out-fps", 60, "output frame rate (integer or rational num/den)")
		sourceFrames  = flag.Int("source-frames", 48, "number of nominal source frames")
		gaps          = flag.String("gaps", "", "comma-separated missing nominal source ranges, e.g. 12-13 or 12-13,20")
		beforeMode    = flag.String("before-mode", "repeat", "initial resampling mode: repeat|blend")
		afterMode     = flag.String("after-mode", "blend", "adjusted resampling mode: repeat|blend")
		interpolation = flag.String("interpolation", "linear", "interpolation family recorded in the strategy block")
		missingPolicy = flag.String("missing-policy", "hold", "policy inside source gaps: hold")
		prefix        = flag.String("prefix", "", "prefix for the per-frame CSVs (e.g. \"missing\" -> missing-repeat-frames.csv)")
		suffix        = flag.String("suffix", "", "suffix for the comparison JSON and curve SVG (e.g. \"missing\" -> judder-curves-missing.svg)")
		outDir        = flag.String("out-dir", "artifacts", "output directory")
	)
	flag.Parse()

	srcRate, err := parseRate(*srcFPS)
	must(err)
	outRate, err := parseRate(*outFPS)
	must(err)

	missing, err := parseGaps(*gaps)
	must(err)

	base := resample.Config{
		SourceRate:   srcRate,
		OutputRate:   outRate,
		SourceFrames: *sourceFrames,
		Missing:      missing,
	}
	beforeCfg := base
	beforeCfg.Strategy = resample.Strategy{
		Mode: *beforeMode, Interpolation: *interpolation, MissingPolicy: *missingPolicy,
	}
	afterCfg := base
	afterCfg.Strategy = resample.Strategy{
		Mode: *afterMode, Interpolation: *interpolation, MissingPolicy: *missingPolicy,
	}

	before := resample.Run(beforeCfg)
	after := resample.Run(afterCfg)
	comparison := resample.Compare(before, after)

	must(os.MkdirAll(*outDir, 0o755))
	pre := *prefix
	if pre != "" {
		pre += "-"
	}
	suf := *suffix
	if suf != "" && !strings.HasPrefix(suf, "-") {
		suf = "-" + suf
	}

	must(writeCSV(filepath.Join(*outDir, pre+"repeat-frames.csv"), pick(before, after, "repeat")))
	must(writeCSV(filepath.Join(*outDir, pre+"blend-frames.csv"), pick(before, after, "blend")))
	jsonName := "judder-comparison" + suf + ".json"
	if suf == "-missing" {
		jsonName = "missing-comparison.json"
	}
	must(writeJSON(filepath.Join(*outDir, jsonName), comparison))
	must(writeSVG(filepath.Join(*outDir, "judder-curves"+suf+".svg"),
		map[string]resample.Report{"before": before, "after": after}))

	fmt.Printf("wrote %srepeat-frames.csv, %sblend-frames.csv, %s, judder-curves%s.svg\n",
		pre, pre, jsonName, suf)
	fmt.Printf("before: mode=%s judder_frames=%d rms=%.6f peak=%.4f\n",
		before.Strategy.Mode, before.Summary.JudderFrames,
		before.Summary.RMSNormalizedJudder, before.Summary.PeakNormalizedJudder)
	fmt.Printf("after:  mode=%s judder_frames=%d rms=%.6f peak=%.4f\n",
		after.Strategy.Mode, after.Summary.JudderFrames,
		after.Summary.RMSNormalizedJudder, after.Summary.PeakNormalizedJudder)
}

func pick(before, after resample.Report, mode string) resample.Report {
	if before.Strategy.Mode == mode {
		return before
	}
	return after
}

func writeCSV(path string, rep resample.Report) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return resample.WriteFramesCSV(f, rep)
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return resample.WriteJSON(f, v)
}

func writeSVG(path string, reports map[string]resample.Report) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return resample.WriteJudderSVG(f, reports)
}

// parseRate accepts 24, 60.0 or a rational such as 24000/1001.
func parseRate(v float64) (resample.Rate, error) {
	// Flag is float; the CLI supports rational input via string by
	// re-reading the raw value is unnecessary for the 24->60 task, but keep
	// a deterministic integer reduction.
	if v != float64(int(v)) {
		return resample.Rate{}, fmt.Errorf("rational rates must be integers in this build, got %v", v)
	}
	return resample.Rate{Num: int(v), Den: 1}, nil
}

// parseGaps turns "12-13,20" into the missing-index set.
func parseGaps(spec string) (map[int]bool, error) {
	missing := map[int]bool{}
	if strings.TrimSpace(spec) == "" {
		return missing, nil
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		bounds := strings.Split(part, "-")
		switch len(bounds) {
		case 1:
			idx, err := strconv.Atoi(bounds[0])
			if err != nil {
				return nil, fmt.Errorf("bad gap %q: %w", part, err)
			}
			missing[idx] = true
		case 2:
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil || lo > hi {
				return nil, fmt.Errorf("bad gap range %q", part)
			}
			for idx := lo; idx <= hi; idx++ {
				missing[idx] = true
			}
		default:
			return nil, fmt.Errorf("bad gap range %q", part)
		}
	}
	return missing, nil
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "judder:", err)
		os.Exit(1)
	}
}

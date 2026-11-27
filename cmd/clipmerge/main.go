package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"clipmerge/internal/conflict"
	"clipmerge/internal/manifest"
	"clipmerge/internal/plan"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "analyze":
		err = runAnalyze(os.Args[2:])
	case "apply":
		err = runApply(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		var codeErr *exitCodeError
		if errors.As(err, &codeErr) {
			fmt.Fprintln(os.Stderr, "clipmerge:", err)
			os.Exit(codeErr.code)
		}
		fmt.Fprintln(os.Stderr, "clipmerge:", err)
		os.Exit(1)
	}
}

type exitCodeError struct {
	code int
	msg  string
}

func (e *exitCodeError) Error() string { return e.msg }

func usage() {
	fmt.Fprint(os.Stderr, `clipmerge - three-layer media identity checker (namespace / UID / hash)

Usage:
  clipmerge analyze -m manifest.json [--media-root DIR] [--report report.json] [--plan plan.json] [--fail-on-conflict]
  clipmerge apply   -m manifest.json -p plan.json -o corrected.json
`)
}

func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	manifestPath := fs.String("m", "", "merged project manifest (JSON)")
	mediaRoot := fs.String("media-root", "", "root directory for on-disk SHA-256 verification")
	reportPath := fs.String("report", "", "write conflict groups JSON here (default: stdout)")
	planPath := fs.String("plan", "", "write rename/rollback plan JSON here")
	failOnConflict := fs.Bool("fail-on-conflict", false, "exit code 2 when conflicts requiring action exist")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *manifestPath == "" {
		return fmt.Errorf("analyze requires -m")
	}

	m, err := manifest.Load(*manifestPath)
	if err != nil {
		return err
	}
	rep := conflict.Analyze(m, *mediaRoot)
	p := plan.Build(m, rep)

	if err := writeJSON(*reportPath, rep); err != nil {
		return err
	}
	if *planPath != "" {
		if err := writeJSON(*planPath, p); err != nil {
			return err
		}
	}

	printSummary(rep, p, *reportPath, *planPath)
	if *failOnConflict && needsAction(rep) {
		return &exitCodeError{code: 2, msg: "conflicts requiring remediation detected"}
	}
	return nil
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	manifestPath := fs.String("m", "", "merged project manifest (JSON)")
	planPath := fs.String("p", "", "plan JSON produced by analyze")
	outputPath := fs.String("o", "corrected-manifest.json", "output path for the corrected manifest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *manifestPath == "" || *planPath == "" {
		return fmt.Errorf("apply requires -m and -p")
	}

	m, err := manifest.Load(*manifestPath)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*planPath)
	if err != nil {
		return fmt.Errorf("read plan: %w", err)
	}
	var p plan.Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("parse plan: %w", err)
	}
	rolled, renamed, err := plan.Apply(m, &p, *outputPath)
	if err != nil {
		return err
	}
	fmt.Printf("applied: %d lock rollback(s), %d clip rename(s); blocked=%d -> %s\n",
		rolled, renamed, len(p.Blocked), *outputPath)
	if len(p.Blocked) > 0 {
		fmt.Fprintln(os.Stderr, "warning: blocked rollbacks remain, inspect the plan before opening the project")
	}
	return nil
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if path == "" {
		_, err = os.Stdout.Write(raw)
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func needsAction(rep *conflict.Report) bool {
	for _, g := range rep.Groups {
		if g.Severity == conflict.SeverityCritical {
			return true
		}
	}
	return false
}

func printSummary(rep *conflict.Report, p *plan.Plan, reportPath, planPath string) {
	counts := map[string]int{}
	for _, g := range rep.Groups {
		counts[g.Kind]++
	}
	fmt.Fprintf(os.Stderr, "conflict groups: %d\n", len(rep.Groups))
	for _, kind := range orderedKinds(counts) {
		fmt.Fprintf(os.Stderr, "  %-20s %d\n", kind, counts[kind])
	}
	fmt.Fprintf(os.Stderr, "plan: %d rename(s), %d rollback(s), %d blocked\n",
		len(p.Renames), len(p.Rollback), len(p.Blocked))
	if reportPath != "" {
		fmt.Fprintf(os.Stderr, "report -> %s\n", reportPath)
	}
	if planPath != "" {
		fmt.Fprintf(os.Stderr, "plan   -> %s\n", planPath)
	}
}

func orderedKinds(counts map[string]int) []string {
	out := make([]string, 0, len(counts))
	for k := range counts {
		out = append(out, k)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// Command lto-manifest builds and verifies content-addressed hand-off
// inventories for media directories destined for LTO archival. It uses only
// the Go standard library and never records absolute paths.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"lto-manifest/internal/budget"
	"lto-manifest/internal/manifest"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "scan":
		err = runScan(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lto-manifest:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `lto-manifest - LTO media hand-off inventory (Go standard library only)

Usage:
  lto-manifest scan   -root DIR [-out manifest.json] [-budget 12TB]
                       [-exclude rel/path]... [-fail-on-budget]
  lto-manifest verify -root DIR -manifest manifest.json
                       [-strict] [-expect-colocation] [-ignore-extras] [-json report.json]

All paths inside the manifest are POSIX-style paths relative to -root.
`)
}

func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	root := fs.String("root", ".", "media directory to inventory")
	out := fs.String("out", "manifest.json", "manifest output path")
	budgetText := fs.String("budget", "", "LTO capacity budget, e.g. 12TB")
	failOnBudget := fs.Bool("fail-on-budget", false, "exit non-zero when total size exceeds budget")
	exclude := multiFlag{}
	fs.Var(&exclude, "exclude", "relative path to skip (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var limit int64
	if *budgetText != "" {
		parsed, err := budget.Parse(*budgetText)
		if err != nil {
			return err
		}
		limit = parsed
	}
	excluded := map[string]bool{}
	for _, e := range exclude {
		excluded[manifest.ToPOSIX(e)] = true
	}

	m, err := manifest.Scan(*root, limit, excluded)
	if err != nil {
		return err
	}
	if err := writeJSON(*out, m); err != nil {
		return err
	}
	printScanSummary(m, *out)
	if *failOnBudget && m.Budget != nil && m.Budget.Over {
		return fmt.Errorf("total %s exceeds budget %s",
			budget.Format(m.Budget.TotalSize), budget.Format(m.Budget.Limit))
	}
	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	root := fs.String("root", ".", "directory to verify against the manifest")
	manifestPath := fs.String("manifest", "manifest.json", "manifest to verify against")
	strict := fs.Bool("strict", false, "fail if any file moved from its manifest path")
	coloc := fs.Bool("expect-colocation", false, "fail if sidecar and media no longer share a directory")
	ignoreExtras := fs.Bool("ignore-extras", false, "allow files not listed in the manifest")
	jsonOut := fs.String("json", "", "write the verification report as JSON to this path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	m, err := manifest.Load(*manifestPath)
	if err != nil {
		return err
	}
	result, err := manifest.Verify(m, *root, manifest.VerifyOptions{
		Strict: *strict, ExpectColoc: *coloc, IgnoreExtras: *ignoreExtras,
	})
	if err != nil {
		return err
	}
	printVerifySummary(result)
	if *jsonOut != "" {
		if err := writeJSON(*jsonOut, result); err != nil {
			return err
		}
	}
	if !result.OK {
		os.Exit(1)
	}
	return nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// multiFlag collects repeated -exclude values.
type multiFlag []string

func (m *multiFlag) String() string { return fmt.Sprint([]string(*m)) }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

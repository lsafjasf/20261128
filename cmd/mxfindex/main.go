package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mxfindex"
)

const usage = `usage:
  mxfindex [-json] [-out report.json] <file.mxf>
  mxfindex scan [-json] [-out report.json] <file.mxf>
  mxfindex rebuild [-json] [-out report.json] <file.mxf>

Exit status: 0 complete, 2 incomplete, 1 error.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
	args := os.Args[1:]
	if args[0] == "scan" || args[0] == "rebuild" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("mxfindex", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOutput := fs.Bool("json", false, "write the full report as JSON")
	outPath := fs.String("out", "", "write the report JSON to this file")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
	input := fs.Arg(0)
	if *outPath != "" {
		inputAbs, err := filepath.Abs(input)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		outAbs, err := filepath.Abs(*outPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if inputAbs == outAbs {
			fmt.Fprintln(os.Stderr, "mxfindex: refusing to overwrite the input MXF")
			os.Exit(1)
		}
	}
	report, err := mxfindex.ScanFile(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *outPath != "" {
		if err := writeJSON(*outPath, report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *jsonOutput {
		if err := encodeIndented(os.Stdout, report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	} else {
		printSummary(report)
	}
	if !report.Success {
		os.Exit(2)
	}
}

func encodeIndented(f *os.File, report *mxfindex.Result) error {
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func writeJSON(path string, report *mxfindex.Result) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".mxfindex-*.json")
	if err != nil {
		return fmt.Errorf("create report: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := encodeIndented(tmp, report); err != nil {
		tmp.Close()
		return fmt.Errorf("write report: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close report: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install report: %w", err)
	}
	return nil
}

func printSummary(report *mxfindex.Result) {
	fmt.Printf("status: %s\n", report.Status)
	fmt.Printf("partitions: %d  index segments: %d  essence elements: %d  timecodes: %d\n",
		len(report.Partitions), len(report.IndexSegments), len(report.Essence), len(report.Timecodes))
	fmt.Printf("rebuilt tables: %d  addressable ranges: %d\n",
		len(report.RebuiltIndex.Tables), len(report.AddressableRanges))
	for _, table := range report.RebuiltIndex.Tables {
		fmt.Printf("IndexSID %d BodySID %d: [%d, %d) %s entries=%d\n",
			table.IndexSID, table.BodySID, table.StartPosition, table.EndPosition,
			table.EditRate, len(table.Entries))
	}
	for _, issue := range report.Issues {
		fmt.Printf("%s %s: %s\n", issue.Severity, issue.Code, issue.Message)
	}
}

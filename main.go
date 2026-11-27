package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"wvcheck/internal/wavpack"
)

type options struct {
	correction string
	json       bool
	verbose    bool
}

func main() {
	var opts options
	flag.StringVar(&opts.correction, "wvc", "", "correction file (default: infer from the .wv path)")
	flag.BoolVar(&opts.json, "json", false, "write machine-readable JSON")
	flag.BoolVar(&opts.verbose, "v", false, "list every main/correction block dependency")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 1 {
		usage()
		os.Exit(1)
	}

	mainPath := flag.Arg(0)
	corrPath, explicit := correctionPath(opts.correction, mainPath)
	mainScan, err := wavpack.ScanFile(mainPath)
	if err != nil {
		fail("main file: %v", err)
	}

	var corrScan *wavpack.ScanResult
	corrExists := false
	if corrPath != "" {
		info, statErr := os.Stat(corrPath)
		switch {
		case statErr == nil && info.IsDir():
			fail("correction path is a directory: %s", corrPath)
		case statErr == nil:
			corrScan, err = wavpack.ScanFile(corrPath)
			if err != nil {
				fail("correction file: %v", err)
			}
			corrExists = true
		case os.IsNotExist(statErr) && !explicit:
			// A missing inferred .wvc is the condition this tool reports.
		case os.IsNotExist(statErr):
			fail("correction file does not exist: %s", corrPath)
		default:
			fail("correction file: %v", statErr)
		}
	}

	result := wavpack.Analyze(mainScan, corrScan)
	result.CorrectionFileExists = corrExists
	result.CorrectionPath = corrPath
	if opts.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fail("encode JSON: %v", err)
		}
	} else {
		printText(result, opts.verbose)
	}
	if !result.Complete {
		os.Exit(2)
	}
}

func correctionPath(requested, mainPath string) (string, bool) {
	if requested != "" {
		return requested, true
	}
	return wavpack.CorrectionPathFor(mainPath), false
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s [-json] [-v] [-wvc correction.wvc] main.wv\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "Reports hybrid ranges that have no usable correction block. It never interpolates samples.")
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "wvcheck: "+format+"\n", args...)
	os.Exit(1)
}

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"releaseaudit"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("release-audit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "package root (default: directory containing the manifest)")
	formatName := fs.String("format", "markdown", "report format: markdown, json, or csv")
	outPath := fs.String("out", "", "write the report to this file instead of stdout")
	skipFiles := fs.Bool("skip-files", false, "skip asset and manifest file existence checks")
	strict := fs.Bool("strict", false, "exit non-zero on warnings as well as errors")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: release-audit [flags] <release.json | ->")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return 2
	}

	manifestPath := ""
	if fs.NArg() == 1 {
		manifestPath = fs.Arg(0)
	} else {
		var err error
		manifestPath, err = findManifest(".")
		if err != nil {
			fmt.Fprintln(stderr, "release-audit:", err)
			return 2
		}
	}
	format, err := releaseaudit.ParseFormat(*formatName)
	if err != nil {
		fmt.Fprintln(stderr, "release-audit:", err)
		return 2
	}

	var pkg *releaseaudit.Package
	packageRoot := *root
	if manifestPath == "-" {
		pkg, err = releaseaudit.Load(os.Stdin)
		if packageRoot == "" {
			packageRoot = "."
		}
	} else {
		pkg, err = releaseaudit.LoadFile(manifestPath)
		if packageRoot == "" {
			packageRoot = filepath.Dir(manifestPath)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "release-audit:", err)
		return 2
	}
	if *skipFiles {
		packageRoot = ""
	}

	report := releaseaudit.Audit(pkg, releaseaudit.Options{Root: packageRoot})
	rendered, err := report.Render(format)
	if err != nil {
		fmt.Fprintln(stderr, "release-audit:", err)
		return 2
	}
	if *outPath != "" {
		if err := os.WriteFile(*outPath, []byte(rendered), 0o644); err != nil {
			fmt.Fprintln(stderr, "release-audit:", err)
			return 2
		}
		fmt.Fprintf(stderr, "wrote %s (%d errors, %d warnings)\n",
			*outPath, report.Summary.Errors, report.Summary.Warnings)
	} else {
		fmt.Fprint(stdout, rendered)
	}
	if report.HasErrors() || (*strict && report.Summary.Warnings > 0) {
		return 1
	}
	return 0
}

func findManifest(dir string) (string, error) {
	for _, name := range []string{"release.json", "release-package.json", "manifest.json"} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	return "", fmt.Errorf("no release manifest found (tried release.json, release-package.json, manifest.json)")
}

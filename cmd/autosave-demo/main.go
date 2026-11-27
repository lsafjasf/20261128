// autosave-demo demonstrates incremental autosave and fail-safe recovery.
package main

import (
	"flag"
	"fmt"
	"os"

	"editorautosave/autosave"
	"editorautosave/internal/demo"
	"editorautosave/recovery"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "demo":
		err = runDemo(os.Args[2:])
	case "inspect":
		err = runInspect(os.Args[2:])
	case "restore":
		err = runRestore(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `autosave-demo - incremental autosave recovery chain demo

usage:
  autosave-demo demo --root DIR
  autosave-demo inspect --recovery DIR [--disk FILE] [--verify-hash]
  autosave-demo restore --recovery DIR --candidate ID [--disk FILE] --out DIR [--verify-hash]
`)
}

func runDemo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	root := fs.String("root", "", "workspace root to create")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return fmt.Errorf("--root is required")
	}
	if err := os.MkdirAll(*root, 0o755); err != nil {
		return err
	}
	summary, err := demo.Run(*root)
	if err != nil {
		return err
	}
	printDemoSummary(summary)
	return nil
}

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	recoveryDir := fs.String("recovery", "", "recovery directory")
	disk := fs.String("disk", "", "on-disk project file (optional)")
	verifyHash := fs.Bool("verify-hash", false, "verify media content hashes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *recoveryDir == "" {
		return fmt.Errorf("--recovery is required")
	}
	store, err := autosave.Open(*recoveryDir)
	if err != nil {
		return err
	}
	report, err := recovery.Inspect(store, *disk, recovery.NewFileVerifier(*verifyHash))
	if err != nil {
		return err
	}
	printReport(report)
	return nil
}

func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	recoveryDir := fs.String("recovery", "", "recovery directory")
	candidate := fs.String("candidate", "", "snapshot id to restore")
	disk := fs.String("disk", "", "original on-disk project file")
	out := fs.String("out", "", "output directory for recovered project")
	verifyHash := fs.Bool("verify-hash", false, "verify media content hashes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *recoveryDir == "" || *candidate == "" || *out == "" {
		return fmt.Errorf("--recovery, --candidate and --out are required")
	}
	store, err := autosave.Open(*recoveryDir)
	if err != nil {
		return err
	}
	result, err := recovery.Restore(store, *candidate, *disk, *out, recovery.NewFileVerifier(*verifyHash))
	if err != nil {
		return err
	}
	fmt.Printf("recovered project: %s (seq %d)\n", result.OutputPath, result.RestoredSeq)
	fmt.Printf("original project:  %s (kept: %t)\n", result.OriginalPath, result.OriginalKept)
	for _, warning := range result.MissingMedia {
		fmt.Printf("media warning: %s\n", warning)
	}
	return nil
}

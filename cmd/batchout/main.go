package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"batchout"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "recover":
		err = recoverCmd(os.Args[2:])
	case "lookup":
		err = lookupCmd(os.Args[2:])
	case "list":
		err = listCmd(os.Args[2:])
	case "verify":
		err = verifyCmd(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "batchout:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  batchout recover -output DIR
  batchout lookup  -output DIR (-file REL | -digest SHA256)
  batchout list    -output DIR
  batchout verify  -output DIR`)
}

func outputDir(fs *flag.FlagSet, args []string) (string, error) {
	dir := fs.String("output", "", "batch output directory")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if *dir == "" {
		return "", errors.New("-output is required")
	}
	return *dir, nil
}

func recoverCmd(args []string) error {
	fs := flag.NewFlagSet("recover", flag.ContinueOnError)
	dir, err := outputDir(fs, args)
	if err != nil {
		return err
	}
	mgr, err := batchout.Open(dir, batchout.WithoutAutoRecover())
	if err != nil {
		return err
	}
	report, recoverErr := mgr.Recover(context.Background())
	for _, entry := range report.Entries {
		fmt.Printf("%-14s %-20s %-10s %s\n", entry.Action, entry.TaskID, entry.State, entry.Journal)
		if entry.Err != nil {
			fmt.Fprintf(os.Stderr, "  error: %v\n", entry.Err)
		}
	}
	return recoverErr
}

func lookupCmd(args []string) error {
	fs := flag.NewFlagSet("lookup", flag.ContinueOnError)
	file := fs.String("file", "", "path relative to output directory")
	digest := fs.String("digest", "", "SHA-256 of the final file")
	dir, err := outputDir(fs, args)
	if err != nil {
		return err
	}
	if (*file == "") == (*digest == "") {
		return errors.New("exactly one of -file or -digest is required")
	}
	mgr, err := batchout.Open(dir)
	if err != nil {
		return err
	}
	var value any
	if *file != "" {
		value, err = mgr.Lookup(*file)
	} else {
		var matches []batchout.FileProvenance
		matches, err = mgr.LookupDigest(*digest)
		if err == nil && len(matches) == 0 {
			err = batchout.ErrNotFound
		}
		value = matches
	}
	if err != nil {
		return err
	}
	return printJSON(value)
}

func listCmd(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	dir, err := outputDir(fs, args)
	if err != nil {
		return err
	}
	mgr, err := batchout.Open(dir)
	if err != nil {
		return err
	}
	manifests, err := mgr.Manifests()
	if err != nil {
		return err
	}
	return printJSON(manifests)
}

func verifyCmd(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	dir, err := outputDir(fs, args)
	if err != nil {
		return err
	}
	mgr, err := batchout.Open(dir)
	if err != nil {
		return err
	}
	manifests, err := mgr.Manifests()
	if err != nil {
		return err
	}
	var errs []error
	for _, manifest := range manifests {
		for _, output := range manifest.Outputs {
			full := filepath.Join(mgr.OutputDir(), filepath.FromSlash(output.Path))
			size, digest, err := digestFile(full)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", output.Path, err))
				continue
			}
			if size != output.Size || digest != output.SHA256 {
				errs = append(errs, fmt.Errorf("%s: content does not match provenance", output.Path))
				continue
			}
			fmt.Printf("ok  %s\n", output.Path)
		}
	}
	return errors.Join(errs...)
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

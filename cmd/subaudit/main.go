// Command subaudit compares subtitle tracks embedded in a release package
// against sidecar subtitle files. Pairing is based on language, content
// hash, timebase and cue count — identical filenames are never trusted.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"subaudit/internal/pair"
	"subaudit/internal/subs"
)

var supportedExt = map[string]bool{
	".srt": true,
	".vtt": true,
	".ass": true,
	".ssa": true,
}

func main() {
	embDir := flag.String("embedded", "", "directory holding embedded-track subtitle extracts")
	sideDir := flag.String("sidecar", "", "directory holding sidecar subtitle files")
	asJSON := flag.Bool("json", false, "emit machine-readable JSON")
	flag.Parse()
	if *embDir == "" || *sideDir == "" {
		fmt.Fprintln(os.Stderr, "usage: subaudit -embedded <dir> -sidecar <dir> [-json]")
		os.Exit(2)
	}

	embDocs, embErrs := loadDir(*embDir)
	sideDocs, sideErrs := loadDir(*sideDir)
	for _, e := range append(embErrs, sideErrs...) {
		fmt.Fprintf(os.Stderr, "warning: %v\n", e)
	}
	if len(embDocs) == 0 || len(sideDocs) == 0 {
		fmt.Fprintf(os.Stderr, "error: need at least one parseable track on each side (embedded=%d sidecar=%d)\n",
			len(embDocs), len(sideDocs))
		os.Exit(2)
	}

	rep := pair.Audit(embDocs, sideDocs)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	} else {
		printReport(rep)
	}

	if rep.Summary.Mismatch > 0 || rep.Summary.Unpaired > 0 {
		os.Exit(1)
	}
}

func loadDir(root string) ([]*subs.Document, []error) {
	var docs []*subs.Document
	var errs []error
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if d.IsDir() || !supportedExt[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		doc, perr := subs.Parse(path)
		if perr != nil {
			errs = append(errs, perr)
			return nil
		}
		docs = append(docs, doc)
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	return docs, errs
}

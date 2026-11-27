package main

import (
	"flag"
	"fmt"
	"os"

	"avsync/av"
	"avsync/gen"
	"avsync/mp4"
)

func usage() {
	fmt.Fprintf(os.Stderr, `avsync - diagnose and fix per-cut audio gaps after enabling B-frames

usage:
  avsync gen  -defect none|reorder|elst|clockreset -out file.mp4 [-shots 4] [-frames 12]
  avsync diag  file.mp4
  avsync fix   file.mp4 -out fixed.mp4
`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "gen":
		cmdGen(os.Args[2:])
	case "diag":
		cmdDiag(os.Args[2:])
	case "fix":
		cmdFix(os.Args[2:])
	default:
		usage()
	}
}

func cmdGen(args []string) {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)
	defect := fs.String("defect", "reorder", "none|reorder|elst|clockreset")
	out := fs.String("out", "out.mp4", "output file")
	shots := fs.Int("shots", 4, "number of shots")
	frames := fs.Int("frames", 12, "frames per shot")
	fs.Parse(args)
	d, ok := gen.ParseDefect(*defect)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown defect %q\n", *defect)
		os.Exit(2)
	}
	if err := gen.Generate(*out, d, *shots, *frames); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (defect=%s, shots=%d, frames/shot=%d)\n", *out, *defect, *shots, *frames)
}

func cmdDiag(args []string) {
	fs := flag.NewFlagSet("diag", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	if _, _, err := diagnoseFile(fs.Arg(0), os.Stdout, true); err != nil {
		fatal(err)
	}
}

func cmdFix(args []string) {
	fs := flag.NewFlagSet("fix", flag.ExitOnError)
	out := fs.String("out", "", "output file (required)")
	fs.Parse(reorderArgs(args))
	if fs.NArg() != 1 || *out == "" {
		usage()
	}
	in := fs.Arg(0)
	f, diag, err := diagnoseFile(in, os.Stdout, true)
	if err != nil {
		fatal(err)
	}
	if diag.Cause == av.CauseNone {
		fmt.Println("nothing to fix")
		return
	}
	tracks, movieTS := av.BuildFixedTracks(f, diag)
	if err := mp4.Mux(*out, tracks, movieTS); err != nil {
		fatal(err)
	}
	fmt.Printf("\nwrote fixed file %s\n", *out)
	fmt.Println("== re-diagnosis of fixed file ==")
	if _, _, err := diagnoseFile(*out, os.Stdout, false); err != nil {
		fatal(err)
	}
}

func diagnoseFile(path string, w *os.File, table bool) (*mp4.File, *av.Diagnosis, error) {
	f, err := mp4.Demux(path)
	if err != nil {
		return nil, nil, err
	}
	m := av.BuildModel(f)
	if m.Video == nil || m.Audio == nil {
		return nil, nil, fmt.Errorf("%s: need one video and one audio track", path)
	}
	t := av.BuildEventTable(m)
	d := av.Diagnose(m, t)
	if table {
		av.RenderTable(w, m, t)
	}
	av.RenderDiagnosis(w, d)
	return f, d, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

// reorderArgs moves positional arguments behind flags so that Go's flag
// package (which stops at the first non-flag argument) parses both.
func reorderArgs(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 0 && a[0] == '-' {
			flags = append(flags, a)
			if !contains(a, '=') && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
		} else {
			positional = append(positional, a)
		}
	}
	return append(flags, positional...)
}

func contains(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

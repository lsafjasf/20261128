package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"mp4safe"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mp4safe:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "inspect":
		return runInspect(args[1:])
	case "strip":
		return runStrip(args[1:])
	case "verify":
		return runVerify(args[1:])
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usageText())
	}
}

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: mp4safe inspect [-json] FILE")
	}
	report, err := mp4safe.InspectFile(fs.Arg(0))
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(os.Stdout, report)
	}
	fmt.Printf("file: %s\nbytes: %d\n", report.File, report.Bytes)
	if len(report.Unsupported) > 0 {
		fmt.Printf("unsupported: %s\n", strings.Join(report.Unsupported, "; "))
	}
	fmt.Println("boxes:")
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "  TYPE\tCLASS\tSTART\tEND\tSIZE\tFLAGS")
	for _, b := range report.Boxes {
		printBox(tw, b, 0)
	}
	tw.Flush()
	fmt.Println("tracks:")
	for _, t := range report.Tracks {
		fmt.Printf("  id=%d handler=%s chunks=%d samples=%d bytes=%d index=%s sha256=%s\n",
			t.ID, t.Handler, t.Chunks, t.Samples, t.Bytes, t.ChunkTable,
			digestForTrack(report, t.ID))
	}
	fmt.Println("unknown boxes:")
	for _, d := range report.Digests.UnknownBoxes {
		fmt.Printf("  %s [%d,%d) bytes=%d sha256=%s\n", d.Path, d.Offset, d.Offset+d.Bytes, d.Bytes, d.SHA256)
	}
	return nil
}

func printBox(tw *tabwriter.Writer, b mp4safe.BoxInfo, depth int) {
	name := strings.Repeat("  ", depth) + b.Type
	flags := ""
	if b.Removable {
		flags = "removable"
	}
	fmt.Fprintf(tw, "  %s\t%s\t%d\t%d\t%d\t%s\n", name, b.Class, b.Start, b.End, b.End-b.Start, flags)
	for _, child := range b.Children {
		printBox(tw, child, depth+1)
	}
}

func digestForTrack(report *mp4safe.Inspection, id uint32) string {
	for _, d := range report.Digests.Tracks {
		if d.TrackID == id {
			return d.SHA256
		}
	}
	return ""
}

func runStrip(args []string) error {
	fs := flag.NewFlagSet("strip", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	in := fs.String("in", "", "input ISO BMFF file")
	out := fs.String("out", "", "output file")
	reportPath := fs.String("report", "", "optional JSON report path")
	jsonOut := fs.Bool("json", false, "emit JSON summary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" || *out == "" || fs.NArg() != 0 {
		return errors.New("usage: mp4safe strip -in INPUT -out OUTPUT [-report REPORT] [-json]")
	}
	result, err := mp4safe.StripPadding(*in, *out)
	if err != nil {
		return err
	}
	if *reportPath != "" {
		if err := writeJSONFile(*reportPath, result); err != nil {
			return err
		}
	}
	if *jsonOut {
		return writeJSON(os.Stdout, result)
	}
	fmt.Printf("input:  %s (%d bytes)\n", result.Input, result.InputBytes)
	fmt.Printf("output: %s (%d bytes)\n", result.Output, result.OutputBytes)
	fmt.Printf("removed padding: %d boxes, %d bytes\n", len(result.RemovedPadding), result.InputBytes-result.OutputBytes)
	fmt.Printf("updated chunk offsets: %d\n", result.UpdatedOffsets)
	fmt.Printf("sample hashes: verified (%d tracks)\n", len(result.After.Tracks))
	fmt.Printf("unknown box bytes: verified (%d boxes)\n", len(result.After.UnknownBoxes))
	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	beforePath := fs.String("before", "", "original file")
	afterPath := fs.String("after", "", "rewritten file")
	jsonOut := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *beforePath == "" || *afterPath == "" || fs.NArg() != 0 {
		return errors.New("usage: mp4safe verify -before INPUT -after OUTPUT [-json]")
	}
	before, err := mp4safe.InspectFile(*beforePath)
	if err != nil {
		return err
	}
	after, err := mp4safe.InspectFile(*afterPath)
	if err != nil {
		return err
	}
	if err := mp4safe.CompareDigests(before.Digests, after.Digests); err != nil {
		return err
	}
	report := struct {
		Verified bool            `json:"verified"`
		Before   mp4safe.Digests `json:"before"`
		After    mp4safe.Digests `json:"after"`
	}{true, before.Digests, after.Digests}
	if *jsonOut {
		return writeJSON(os.Stdout, report)
	}
	fmt.Printf("verified: samples, mdat payloads, and unknown boxes match\n")
	for _, d := range after.Digests.Tracks {
		fmt.Printf("track %d (%s): %d samples, %d bytes, sha256=%s\n",
			d.TrackID, d.Handler, d.Samples, d.Bytes, d.SHA256)
	}
	fmt.Printf("unknown boxes: %d, mdat payloads: %d\n",
		len(after.Digests.UnknownBoxes), len(after.Digests.MediaPayloads))
	return nil
}

func writeJSON(w *os.File, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write report %s: %w", path, err)
	}
	return nil
}

func usageError() error {
	printUsage(os.Stderr)
	return errors.New("a command is required")
}

func usageText() string {
	return `usage:
  mp4safe inspect [-json] FILE
  mp4safe strip -in INPUT -out OUTPUT [-report REPORT] [-json]
  mp4safe verify -before INPUT -after OUTPUT [-json]`
}

func printUsage(w *os.File) {
	fmt.Fprintln(w, usageText())
}

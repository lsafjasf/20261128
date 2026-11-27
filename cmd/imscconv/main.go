package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"imscconv/imsc"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "imscconv:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("imscconv", flag.ContinueOnError)
	in := fs.String("in", "", "input IMSC TTML file (default: stdin)")
	out := fs.String("out", "", "output IMSC TTML file (default: stdout)")
	mode := fs.String("mode", "embedded", "resource mode: embedded|datauri")
	verify := fs.Bool("verify", false, "re-parse the exported document and print reference statistics")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var reader io.Reader = os.Stdin
	if *in != "" {
		f, err := os.Open(*in)
		if err != nil {
			return err
		}
		defer f.Close()
		reader = f
	}

	doc, err := imsc.Parse(reader)
	if err != nil {
		return err
	}

	opts := imsc.DefaultExportOptions()
	if *mode == "datauri" {
		opts = imsc.ExportOptions{DataURI: true}
	}

	var writer io.Writer = os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		writer = f
	}

	if !*verify {
		return doc.Export(writer, opts)
	}

	// Export to a buffer, re-read it, then write the re-parsed document so
	// the reference relationships are proven to survive a full round trip.
	buf := &bytes.Buffer{}
	if err := doc.Export(buf, opts); err != nil {
		return err
	}
	reparsed, err := imsc.Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return fmt.Errorf("round-trip parse failed: %w", err)
	}
	if err := reparsed.Export(writer, opts); err != nil {
		return err
	}
	printStats(doc, reparsed)
	return nil
}

func printStats(first, second *imsc.Document) {
	fmt.Fprintf(os.Stderr, "parse:    cues=%d resources=%d regions=%d conflicts=%d\n",
		first.CueCount(), first.Resources.Len(), first.Regions.Len(), len(first.Conflicts))
	fmt.Fprintf(os.Stderr, "reparse:  cues=%d resources=%d regions=%d conflicts=%d\n",
		second.CueCount(), second.Resources.Len(), second.Regions.Len(), len(second.Conflicts))
	for _, c := range first.Conflicts {
		fmt.Fprintf(os.Stderr, "conflict: uri=%q kept=%s rejected=%s (%s vs %s)\n",
			c.URI, short(c.FirstKey), short(c.ConflictingKey), c.FirstMediaType, c.SecondMediaType)
	}
}

func short(key string) string {
	if len(key) > 12 {
		return key[:12]
	}
	return key
}

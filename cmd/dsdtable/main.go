package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"dsdtable/dsd"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "dsdtable:", err)
		os.Exit(1)
	}
}

func run() error {
	jsonOut := flag.Bool("json", false, "write the full manifest as JSON")
	extractDir := flag.String("extract", "", "write valid packed samples to chN.raw files in this directory")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [-json] [-extract DIR] FILE|-\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		return errors.New("exactly one DFF or DSF file is required")
	}

	path := flag.Arg(0)
	manifest, reader, closeReader, err := load(path)
	if err != nil {
		return err
	}
	if closeReader != nil {
		defer closeReader()
	}
	if *jsonOut {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(manifest); err != nil {
			return err
		}
	} else {
		printManifest(os.Stdout, manifest)
	}
	if *extractDir != "" {
		if err := extract(manifest, reader, path, *extractDir); err != nil {
			return err
		}
	}
	return nil
}

func load(path string) (*dsd.Manifest, io.ReaderAt, func(), error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, nil, nil, err
		}
		manifest, err := dsd.ParseBytes(data)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("stdin: %w", err)
		}
		return manifest, bytes.NewReader(data), nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, nil, err
	}
	manifest, err := dsd.Parse(file, info.Size())
	if err != nil {
		file.Close()
		return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	manifest.Path = path
	return manifest, file, func() { file.Close() }, nil
}

func extract(manifest *dsd.Manifest, reader io.ReaderAt, source, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	base := "stdin"
	if source != "-" {
		base = strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	}
	for i := range manifest.ChannelTables {
		data, err := manifest.ExtractChannel(reader, i)
		if err != nil {
			return err
		}
		name := fmt.Sprintf("%s.ch%d.raw", base, i)
		target := filepath.Join(dir, name)
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s (%d valid bytes)\n", target, len(data))
	}
	return nil
}

package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"assvtt/ass"
)

func main() {
	sample := flag.Duration("sample", ass.DefaultSampleInterval, "static segment duration while a transform is animating")
	outputPath := flag.String("o", "", "output file (default stdout)")
	flag.Parse()

	input, closeInput, err := openInput(flag.Arg(0))
	if err != nil {
		fatal(err)
	}
	defer closeInput()

	document, err := ass.Convert(input, ass.Options{SampleInterval: *sample})
	if err != nil {
		fatal(err)
	}
	if *outputPath == "" {
		if _, err := io.WriteString(os.Stdout, document); err != nil {
			fatal(err)
		}
		return
	}
	if err := os.WriteFile(*outputPath, []byte(document), 0o644); err != nil {
		fatal(err)
	}
}

func openInput(path string) (io.Reader, func(), error) {
	if path == "" || path == "-" {
		return os.Stdin, func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	return file, func() { _ = file.Close() }, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ass2vtt:", err)
	os.Exit(1)
}

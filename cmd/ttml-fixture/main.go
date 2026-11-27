package main

import (
	"flag"
	"fmt"
	"os"

	"ttmlruby/internal/testfixture"
)

func main() {
	dir := flag.String("out", "testdata/generated", "output directory")
	flag.Parse()
	if err := testfixture.WriteAll(*dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote generated TTML fixtures to %s\n", *dir)
}

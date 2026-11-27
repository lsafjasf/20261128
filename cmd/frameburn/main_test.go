package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateAndVerifyCommands(t *testing.T) {
	dir := t.TempDir()
	args := []string{
		"-out", dir, "-rate", "29.97DF", "-width", "240", "-height", "135",
		"-frames", "30", "-pause", "8:3", "-black", "16:2", "-cut", "22", "-quiet",
	}
	if err := runGenerate(args); err != nil {
		t.Fatal(err)
	}
	if err := runVerify([]string{"-dir", dir, "-quiet"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.json")); err != nil {
		t.Fatal(err)
	}
}

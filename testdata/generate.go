// Command generate writes the deterministic Y4M sample used by the README.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"ghostdiag/internal/synth"
)

func main() {
	seq := synth.Build()
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	testdata := filepath.Join(dir, "testdata")
	if err := os.MkdirAll(testdata, 0o755); err != nil {
		panic(err)
	}
	writeY4M(filepath.Join(testdata, "original.y4m"), seq, seq.Original)
	writeY4M(filepath.Join(testdata, "denoised.y4m"), seq, seq.Denoised)
	data, err := json.MarshalIndent(seq.Trajectory, "", "  ")
	if err != nil {
		panic(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(testdata, "trajectory.json"), data, 0o644); err != nil {
		panic(err)
	}
	fmt.Println("wrote testdata/original.y4m, testdata/denoised.y4m and testdata/trajectory.json")
}

func writeY4M(path string, seq synth.Sequence, frames [][]byte) {
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	if err := synth.WriteY4M(file, seq, frames); err != nil {
		_ = file.Close()
		panic(err)
	}
	if err := file.Close(); err != nil {
		panic(err)
	}
}

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
)

type Report struct {
	File       string            `json:"file"`
	SampleRate int               `json:"sample_rate"`
	Channels   []ChannelAnalysis `json:"channels"`
	Note       string            `json:"note,omitempty"`
}

type FixReport struct {
	Input    string          `json:"input"`
	Output   string          `json:"output"`
	Repairs  []ChannelRepair `json:"repairs"`
	PostFix  []ChannelAnalysis `json:"post_fix_analysis"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "analyze":
		cmdAnalyze(os.Args[2:])
	case "fix":
		cmdFix(os.Args[2:])
	case "demo":
		cmdDemo(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `dcfix - DC offset / drift / subsonic analyzer and repair tool

usage:
  dcfix analyze  <in.wav> [-json report.json]
  dcfix fix      <in.wav> <out.wav> [-mode drift|fixed|subsonic] [-keep-subsonic] [-json fix.json] [-float]
  dcfix demo     <out.wav>            # synthesize a DC-biased drifting test file`)
}

func cmdAnalyze(args []string) {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	jsonOut := fs.String("json", "", "write JSON report to file")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
		os.Exit(2)
	}
	w, err := ReadWav(fs.Arg(0))
	must(err)
	rep := analyzeAll(fs.Arg(0), w)
	emit(rep, *jsonOut)
}

func cmdFix(args []string) {
	fs := flag.NewFlagSet("fix", flag.ExitOnError)
	mode := fs.String("mode", "drift", "fixed|drift|subsonic")
	keepSub := fs.Bool("keep-subsonic", true, "preserve genuine <20Hz content")
	jsonOut := fs.String("json", "", "write JSON fix report (per-segment diffs)")
	forceFloat := fs.Bool("float", true, "write 32-bit float output")
	fs.Parse(args)
	if fs.NArg() != 2 {
		usage()
		os.Exit(2)
	}
	in, out := fs.Arg(0), fs.Arg(1)
	w, err := ReadWav(in)
	must(err)

	frep := FixReport{Input: in, Output: out}
	for c := 0; c < w.Channels; c++ {
		frep.Repairs = append(frep.Repairs,
			RepairChannel(c, w.Samples[c], w.SampleRate, RepairMode(*mode), *keepSub))
	}
	must(WriteWav(out, w, *forceFloat))
	for c := 0; c < w.Channels; c++ {
		frep.PostFix = append(frep.PostFix, AnalyzeChannel(c, w.Samples[c], w.SampleRate))
	}
	emit(frep, *jsonOut)
}

func analyzeAll(name string, w *WavFile) Report {
	rep := Report{File: name, SampleRate: w.SampleRate}
	for c := 0; c < w.Channels; c++ {
		rep.Channels = append(rep.Channels, AnalyzeChannel(c, w.Samples[c], w.SampleRate))
	}
	rep.Note = "headroom_pos/neg_db: margin to +FS/-FS; effective_headroom_db is the worse side (what a peak meter hides)"
	return rep
}

func emit(v any, jsonPath string) {
	var data []byte
	var err error
	data, err = json.MarshalIndent(v, "", "  ")
	must(err)
	if jsonPath != "" {
		must(os.WriteFile(jsonPath, data, 0o644))
		fmt.Fprintf(os.Stderr, "report written to %s\n", jsonPath)
		return
	}
	os.Stdout.Write(append(data, '\n'))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// cmdDemo synthesizes a stereo file: ch0 fixed DC + drift, ch1 fixed DC +
// genuine 15Hz subsonic, both with program material. Peaks stay just under
// FS so downstream gain would clip on the biased side only.
func cmdDemo(args []string) {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	dur := fs.Float64("dur", 4.0, "seconds")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
		os.Exit(2)
	}
	sr := 48000
	n := int(*dur * float64(sr))
	ch0 := make([]float64, n)
	ch1 := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		prog := 0.55 * math.Sin(2*math.Pi*220*t) * (0.6 + 0.4*math.Sin(2*math.Pi*0.5*t))
		drift := 0.12 * (t / *dur) // gradual drift 0 -> 0.12
		ch0[i] = prog + 0.08 + drift
		sub := 0.05 * math.Sin(2*math.Pi*15*t) // genuine subsonic
		ch1[i] = prog*0.9 - 0.06 + sub
	}
	w := &WavFile{SampleRate: sr, Channels: 2, Samples: [][]float64{ch0, ch1}, SourceBits: 16}
	must(WriteWav(fs.Arg(0), w, false))
	fmt.Fprintf(os.Stderr, "demo written: ch0=+0.08 fixed + 0.12 drift, ch1=-0.06 fixed + 15Hz subsonic\n")
}

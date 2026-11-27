// sf2check parses a SoundFont 2 file, dumps resolved zones (key ranges,
// loops, envelopes) and reports validation issues such as uncovered keys
// and out-of-bounds loop points.
package main

import (
	"flag"
	"fmt"
	"os"

	"sf2"
)

func main() {
	verbose := flag.Bool("v", false, "dump resolved zones")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: sf2check [-v] <file.sf2>")
		os.Exit(2)
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	font, err := sf2.Parse(f, st.Size())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s (SF %d.%d, %s): %d presets, %d instruments, %d samples\n",
		font.BankName, font.Version[0], font.Version[1], font.SoundEngine,
		len(font.Presets), len(font.Instruments), len(font.Samples))

	if *verbose {
		for _, p := range font.Presets {
			for _, rz := range font.ResolvePreset(p) {
				fmt.Printf("preset %d/%d %q | inst %q | sample %q\n",
					p.Bank, p.Number, p.Name, rz.Instrument.Name, rz.Sample.Name)
				fmt.Printf("  keys %d-%d vel %d-%d loop [%d, %d) mode %d\n",
					rz.KeyLo, rz.KeyHi, rz.VelLo, rz.VelHi,
					rz.LoopStart, rz.LoopEnd, rz.LoopMode)
				fmt.Printf("  volEnv %+v\n  modEnv %+v\n", rz.VolEnv, rz.ModEnv)
				for _, g := range rz.Generators {
					if !g.Known() {
						fmt.Printf("  unknown generator op=%d value=0x%04X (preserved)\n", g.Op, g.Value)
					}
				}
			}
		}
	}

	issues := font.Validate()
	errs := 0
	for _, i := range issues {
		fmt.Println(i)
		if i.Severity == sf2.Error {
			errs++
		}
	}
	if len(issues) == 0 {
		fmt.Println("no issues found")
	}
	if errs > 0 {
		os.Exit(1)
	}
}

// fieldcut demonstrates the joint field-order / cut-point / output-rate
// model: it takes a shot list in source field units, classifies every cut,
// repairs the ones that would flicker, and verifies the result field by
// field.
package main

import (
	"flag"
	"fmt"
	"os"

	"fieldcut"
)

func main() {
	mode := flag.String("mode", "interlaced", "output mode: interlaced|progressive-frame|progressive-field")
	verbose := flag.Bool("v", false, "print field-by-field timestamp table")
	flag.Parse()

	src := fieldcut.Source{FieldRate: fieldcut.R(60000, 1001), Order: fieldcut.TopFirst}
	var out fieldcut.OutputSpec
	switch *mode {
	case "interlaced":
		out = fieldcut.OutputSpec{Kind: fieldcut.InterlacedOut, Rate: fieldcut.R(30000, 1001), Order: fieldcut.TopFirst}
	case "progressive-frame":
		out = fieldcut.OutputSpec{Kind: fieldcut.ProgressiveFrameRateOut, Rate: fieldcut.R(30000, 1001)}
	case "progressive-field":
		out = fieldcut.OutputSpec{Kind: fieldcut.ProgressiveFieldRateOut, Rate: fieldcut.R(60000, 1001)}
	default:
		fmt.Fprintln(os.Stderr, "unknown mode:", *mode)
		os.Exit(2)
	}

	// Two shots, both trimmed at non-field-pair boundaries.
	cuts := []fieldcut.Cut{{In: 0, Out: 7}, {In: 100, Out: 107}}

	fmt.Printf("source: %s fields/s (%s), output: %s fps\n\n", src.FieldRate, src.Order, out.Rate)

	fmt.Println("-- before fix (cuts applied verbatim) --")
	raw := make([]fieldcut.Decision, len(cuts))
	for i, c := range cuts {
		raw[i] = fieldcut.Decision{Cut: c}
	}
	rawFields := fieldcut.Emit(raw, src, out)
	report(fieldcut.VerifyFields(rawFields, src, out), *verbose, rawFields)

	fmt.Println("\n-- joint model decisions --")
	ds := fieldcut.NormalizeTimeline(cuts, src, out)
	for i, d := range ds {
		fmt.Printf("shot %d: %-14s [%d,%d)  %s\n", i, d.Action, d.Cut.In, d.Cut.Out, d.Reason)
	}

	fmt.Println("\n-- after fix --")
	fixed := fieldcut.Emit(ds, src, out)
	report(fieldcut.VerifyFields(fixed, src, out), *verbose, fixed)
}

func report(issues []fieldcut.Issue, verbose bool, fields []fieldcut.OutField) {
	if len(issues) == 0 {
		fmt.Printf("OK: %d fields, timestamps and pair integrity verified\n", len(fields))
	} else {
		for _, is := range issues {
			fmt.Printf("FAIL seq=%d %-14s %s\n", is.Seq, is.Kind, is.Detail)
		}
	}
	if verbose {
		for _, f := range fields {
			fmt.Printf("  seq=%-3d src_field=%-4d seg=%d pts=%-12s (%.4fs) %s\n",
				f.Seq, f.SourceFld, f.Segment, f.PTS, f.PTS.Float(), f.Dominance)
		}
	}
}

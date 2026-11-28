package fieldcut

import "testing"

var (
	pal5994 = Source{FieldRate: R(60000, 1001), Order: TopFirst}
	outI    = OutputSpec{Kind: InterlacedOut, Rate: R(30000, 1001), Order: TopFirst}
	outP30  = OutputSpec{Kind: ProgressiveFrameRateOut, Rate: R(30000, 1001)}
	outP60  = OutputSpec{Kind: ProgressiveFieldRateOut, Rate: R(60000, 1001)}
)

func TestOddTrimLegalAtFieldRateOutput(t *testing.T) {
	d := NormalizeCut(Cut{In: 101, Out: 257}, pal5994, outP60)
	if d.Action != KeepOddTrim {
		t.Fatalf("got %v, want keep-odd-trim", d.Action)
	}
	if d.Cut.In != 101 || d.Cut.Out != 257 {
		t.Fatalf("cut was modified: %+v", d.Cut)
	}
}

func TestOddTrimMustExpandForInterlacedOutput(t *testing.T) {
	d := NormalizeCut(Cut{In: 101, Out: 257}, pal5994, outI)
	if d.Action != ExpandToPair {
		t.Fatalf("got %v, want expand-to-pair", d.Action)
	}
	if d.Cut.In != 100 || d.Cut.Out != 258 {
		t.Fatalf("got [%d,%d), want [100,258)", d.Cut.In, d.Cut.Out)
	}
}

func TestOddTrimMustExpandForProgressiveFrameRate(t *testing.T) {
	d := NormalizeCut(Cut{In: 3, Out: 9}, pal5994, outP30)
	if d.Action != ExpandToPair || d.Cut.In != 2 || d.Cut.Out != 10 {
		t.Fatalf("got %v %+v", d.Action, d.Cut)
	}
}

func TestPairAlignedCutUntouched(t *testing.T) {
	for _, out := range []OutputSpec{outI, outP30, outP60} {
		d := NormalizeCut(Cut{In: 100, Out: 258}, pal5994, out)
		if d.Action != KeepPair {
			t.Fatalf("rate %s: got %v, want keep-pair", out.Rate, d.Action)
		}
	}
}

func TestResampledOutputAllowsOddTrim(t *testing.T) {
	out24 := OutputSpec{Kind: ResampledOut, Rate: R(24, 1)}
	d := NormalizeCut(Cut{In: 7, Out: 33}, pal5994, out24)
	if d.Action != KeepOddTrim {
		t.Fatalf("got %v, want keep-odd-trim (resample regenerates timestamps)", d.Action)
	}
}

func TestTimelineExpansionMergesOverlap(t *testing.T) {
	// Two shots whose expansions would collide on the shared pair.
	cuts := []Cut{{In: 1, Out: 9}, {In: 9, Out: 15}}
	ds := NormalizeTimeline(cuts, pal5994, outI)
	if len(ds) != 2 {
		t.Fatalf("want 2 decisions, got %d", len(ds))
	}
	if ds[0].Cut.Out != ds[1].Cut.In {
		t.Fatalf("segments not contiguous: %+v / %+v", ds[0].Cut, ds[1].Cut)
	}
	// No source field may be emitted twice.
	seen := map[int64]bool{}
	for _, d := range ds {
		for f := d.Cut.In; f < d.Cut.Out; f++ {
			if seen[f] {
				t.Fatalf("field %d emitted twice", f)
			}
			seen[f] = true
		}
	}
}

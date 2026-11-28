package fieldcut

import "testing"

// Reproduces the reported bug: shot A trimmed to an odd field count, then
// shot B appended — the join frame weaves A's last field with B's first.
func TestFlickerDetectedAtOddJoin(t *testing.T) {
	raw := []Decision{
		{KeepPair, Cut{In: 0, Out: 7}, ""}, // 7 fields: odd!
		{KeepPair, Cut{In: 100, Out: 108}, ""},
	}
	fields := Emit(raw, pal5994, outI)
	issues := VerifyFields(fields, pal5994, outI)
	var pair, dominance bool
	for _, is := range issues {
		if is.Kind == "pair-integrity" {
			pair = true
		}
		if is.Kind == "dominance" {
			dominance = true
		}
	}
	if !pair || !dominance {
		t.Fatalf("flicker not detected: %+v", issues)
	}
}

// After NormalizeTimeline expands the cuts, the same join must be clean.
func TestFlickerFixedAfterExpansion(t *testing.T) {
	ds := NormalizeTimeline([]Cut{{In: 0, Out: 7}, {In: 100, Out: 107}}, pal5994, outI)
	fields := Emit(ds, pal5994, outI)
	if issues := VerifyFields(fields, pal5994, outI); len(issues) != 0 {
		t.Fatalf("clean output expected, got %+v", issues)
	}
}

// Field-rate output: odd trims are legal, timestamps must stay uniform.
func TestFieldRateOutputTimestampsUniform(t *testing.T) {
	ds := NormalizeTimeline([]Cut{{In: 3, Out: 11}, {In: 51, Out: 60}}, pal5994, outP60)
	fields := Emit(ds, pal5994, outP60)
	if len(fields) != 17 {
		t.Fatalf("want 17 output fields, got %d", len(fields))
	}
	fdur := pal5994.FieldDuration()
	for i, f := range fields {
		want := fdur.Scale(int64(i))
		if !f.PTS.Equal(want) {
			t.Fatalf("field %d PTS %s, want %s", i, f.PTS, want)
		}
	}
	if issues := VerifyFields(fields, pal5994, outP60); len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
}

// A tampered PTS (one duplicated field timestamp) must be caught.
func TestPTSGapDetected(t *testing.T) {
	ds := NormalizeTimeline([]Cut{{In: 0, Out: 8}}, pal5994, outI)
	fields := Emit(ds, pal5994, outI)
	fields[4].PTS = fields[3].PTS // duplicate timestamp
	issues := VerifyFields(fields, pal5994, outI)
	var pts bool
	for _, is := range issues {
		if is.Kind == "pts" {
			pts = true
		}
	}
	if !pts {
		t.Fatalf("PTS violation not detected: %+v", issues)
	}
}

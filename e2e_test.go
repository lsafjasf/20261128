package main

import (
	"path/filepath"
	"testing"

	"avsync/av"
	"avsync/gen"
	"avsync/mp4"
)

func diagnose(t *testing.T, path string) (*mp4.File, *av.Diagnosis) {
	t.Helper()
	f, err := mp4.Demux(path)
	if err != nil {
		t.Fatalf("demux %s: %v", path, err)
	}
	m := av.BuildModel(f)
	return f, av.Diagnose(m, av.BuildEventTable(m))
}

func TestDiagnoseAllDefects(t *testing.T) {
	cases := []struct {
		defect gen.Defect
		want   av.Cause
	}{
		{gen.DefectNone, av.CauseNone},
		{gen.DefectReorderWait, av.CauseReorderWait},
		{gen.DefectEditList, av.CauseEditList},
		{gen.DefectClockReset, av.CauseClockReset},
	}
	for _, tc := range cases {
		path := filepath.Join(t.TempDir(), "in.mp4")
		if err := gen.Generate(path, tc.defect, 4, 12); err != nil {
			t.Fatal(err)
		}
		_, diag := diagnose(t, path)
		if diag.Cause != tc.want {
			t.Errorf("defect %d: got cause %v, want %v", tc.defect, diag.Cause, tc.want)
		}
	}
}

func TestFixResolvesAllDefects(t *testing.T) {
	for _, defect := range []gen.Defect{gen.DefectReorderWait, gen.DefectEditList, gen.DefectClockReset} {
		dir := t.TempDir()
		in := filepath.Join(dir, "in.mp4")
		out := filepath.Join(dir, "out.mp4")
		if err := gen.Generate(in, defect, 4, 12); err != nil {
			t.Fatal(err)
		}
		f, diag := diagnose(t, in)
		tracks, movieTS := av.BuildFixedTracks(f, diag)
		if err := mp4.Mux(out, tracks, movieTS); err != nil {
			t.Fatal(err)
		}
		if _, diag2 := diagnose(t, out); diag2.Cause != av.CauseNone {
			t.Errorf("defect %d: after fix cause = %v, want none", defect, diag2.Cause)
		}
	}
}

// TestReorderFixIsNotGlobalDelay verifies the reorder-wait fix shifts audio
// per cut (cumulative, growing correction) instead of applying one constant
// offset to the whole track.
func TestReorderFixIsNotGlobalDelay(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
	out := filepath.Join(dir, "out.mp4")
	if err := gen.Generate(in, gen.DefectReorderWait, 4, 12); err != nil {
		t.Fatal(err)
	}
	f, diag := diagnose(t, in)
	if diag.Cause != av.CauseReorderWait {
		t.Fatalf("precondition: cause = %v", diag.Cause)
	}
	tracks, movieTS := av.BuildFixedTracks(f, diag)
	if err := mp4.Mux(out, tracks, movieTS); err != nil {
		t.Fatal(err)
	}
	before, _ := diagnose(t, in)
	after, _ := diagnose(t, out)
	aBefore := audioTrack(t, before)
	aAfter := audioTrack(t, after)
	if len(aBefore.Samples) != len(aAfter.Samples) {
		t.Fatalf("sample count changed: %d -> %d", len(aBefore.Samples), len(aAfter.Samples))
	}
	// The first samples must be untouched: a global delay would move them.
	if aAfter.Samples[0].DTS != aBefore.Samples[0].DTS {
		t.Errorf("first audio sample moved: %d -> %d (looks like a global delay)",
			aBefore.Samples[0].DTS, aAfter.Samples[0].DTS)
	}
	// The correction must grow at each cut, not stay constant.
	n := len(aBefore.Samples)
	corr := func(i int) int64 { return aBefore.Samples[i].DTS - aAfter.Samples[i].DTS }
	c1, c2, c3 := corr(n/4), corr(n/2), corr(n-1)
	if !(c1 < c2 && c2 < c3) {
		t.Errorf("correction not growing per cut: %d, %d, %d", c1, c2, c3)
	}
	if c3 != 3*3840 { // 3 cuts x 80 ms in 48 kHz ticks
		t.Errorf("final correction = %d, want %d", c3, 3*3840)
	}
}

// TestNonAudioDefectsKeepAudioUntouched verifies that edit-list and
// clock-reset fixes never move a single audio sample.
func TestNonAudioDefectsKeepAudioUntouched(t *testing.T) {
	for _, defect := range []gen.Defect{gen.DefectEditList, gen.DefectClockReset} {
		dir := t.TempDir()
		in := filepath.Join(dir, "in.mp4")
		out := filepath.Join(dir, "out.mp4")
		if err := gen.Generate(in, defect, 4, 12); err != nil {
			t.Fatal(err)
		}
		f, diag := diagnose(t, in)
		tracks, movieTS := av.BuildFixedTracks(f, diag)
		if err := mp4.Mux(out, tracks, movieTS); err != nil {
			t.Fatal(err)
		}
		before, _ := diagnose(t, in)
		after, _ := diagnose(t, out)
		aBefore := audioTrack(t, before)
		aAfter := audioTrack(t, after)
		for i := range aBefore.Samples {
			if aBefore.Samples[i].DTS != aAfter.Samples[i].DTS {
				t.Fatalf("defect %d: audio sample %d moved %d -> %d",
					defect, i, aBefore.Samples[i].DTS, aAfter.Samples[i].DTS)
			}
		}
	}
}

// TestPayloadsPreserved checks the remux keeps every sample payload intact.
func TestPayloadsPreserved(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
	out := filepath.Join(dir, "out.mp4")
	if err := gen.Generate(in, gen.DefectClockReset, 4, 12); err != nil {
		t.Fatal(err)
	}
	f, diag := diagnose(t, in)
	tracks, movieTS := av.BuildFixedTracks(f, diag)
	if err := mp4.Mux(out, tracks, movieTS); err != nil {
		t.Fatal(err)
	}
	before, _ := diagnose(t, in)
	after, _ := diagnose(t, out)
	for ti := range before.Tracks {
		tb, ta := before.Tracks[ti], after.Tracks[ti]
		if len(tb.Samples) != len(ta.Samples) {
			t.Fatalf("track %d: sample count changed", ti)
		}
		for i := range tb.Samples {
			db := before.Data(tb, tb.Samples[i])
			da := after.Data(ta, ta.Samples[i])
			if len(db) != len(da) {
				t.Fatalf("track %d sample %d: payload size %d -> %d", ti, i, len(db), len(da))
			}
			for j := range db {
				if db[j] != da[j] {
					t.Fatalf("track %d sample %d: payload byte %d changed", ti, i, j)
				}
			}
		}
	}
}

func audioTrack(t *testing.T, f *mp4.File) *mp4.Track {
	t.Helper()
	for _, tr := range f.Tracks {
		if tr.Handler == "soun" {
			return tr
		}
	}
	t.Fatal("no audio track")
	return nil
}

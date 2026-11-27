package main

import (
	"math"
	"testing"
)

func TestSubsonicPureDrift(t *testing.T) {
	sr := 48000
	n := sr / 2 // one segment
	x := make([]float64, n)
	for i := range x {
		t := float64(i) / float64(sr)
		x[i] = 0.14 + 0.03*t // DC + linear drift only
	}
	got := subsonicRMS(x, sr)
	if got > 1e-4 {
		t.Errorf("pure drift leaked into subsonic: %g", got)
	}
}

func TestSubsonicTone(t *testing.T) {
	sr := 48000
	n := sr / 2
	x := make([]float64, n)
	for i := range x {
		t := float64(i) / float64(sr)
		x[i] = 0.5 * math.Sin(2*math.Pi*220*t)
	}
	got := subsonicRMS(x, sr)
	if got > 1e-3 {
		t.Errorf("220Hz tone leaked into subsonic: %g", got)
	}
}

func TestSubsonicReal15Hz(t *testing.T) {
	sr := 48000
	n := sr / 2
	x := make([]float64, n)
	for i := range x {
		t := float64(i) / float64(sr)
		x[i] = 0.05 * math.Sin(2*math.Pi*15*t)
	}
	got := subsonicRMS(x, sr)
	want := 0.05 / math.Sqrt2
	if math.Abs(got-want) > want*0.3 {
		t.Errorf("15Hz subsonic misestimated: got %g want ~%g", got, want)
	}
}

func TestRepairRemovesFixedDC(t *testing.T) {
	sr := 48000
	n := sr * 2
	x := make([]float64, n)
	for i := range x {
		t := float64(i) / float64(sr)
		x[i] = 0.3*math.Sin(2*math.Pi*100*t) + 0.08
	}
	rep := RepairChannel(0, x, sr, ModeDrift, false)
	var sum float64
	for _, v := range x {
		sum += v
	}
	if mean := sum / float64(n); math.Abs(mean) > 1e-3 {
		t.Errorf("residual DC after repair: %g", mean)
	}
	if len(rep.Segments) != 4 {
		t.Errorf("want 4 segment diffs, got %d", len(rep.Segments))
	}
	for _, s := range rep.Segments {
		if s.Changed == 0 {
			t.Errorf("segment %d records no changed samples", s.Index)
		}
	}
}

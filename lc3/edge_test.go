package lc3

import "testing"

func TestEmptyAndSingle(t *testing.T) {
	tl, err := Build(48000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tl.TotalSamples() != 0 || len(tl.Gaps()) != 0 {
		t.Fatal("empty stream should be zero length")
	}
	one, err := Build(8000, []Frame{frame(0, Dur7p5ms, 20)})
	if err != nil {
		t.Fatal(err)
	}
	if one.TotalSamples() != 60 || one.Frames()[0].Length() != 60 {
		t.Fatalf("single 7.5ms@8k frame = %d, want 60", one.TotalSamples())
	}
}

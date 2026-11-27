package timebase

import "testing"

func TestConvertRat(t *testing.T) {
	cases := []struct {
		name             string
		tick             int64
		src, dst         Timebase
		wantNum, wantDen int64
		wantFloor        int64
		wantCeil         int64
	}{
		{"even fine -> coarse", 240, Fine5994, Coarse2997, 120, 1, 120, 120},
		{"odd fine -> half frame", 241, Fine5994, Coarse2997, 241, 2, 120, 121},
		{"zero", 0, Fine5994, Coarse2997, 0, 1, 0, 0},
		{"coarse -> fine", 120, Coarse2997, Fine5994, 240, 1, 240, 240},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ConvertRat(tc.tick, tc.src, tc.dst)
			if got.Num != tc.wantNum || got.Den != tc.wantDen {
				t.Fatalf("ConvertRat = %s, want %d/%d", got, tc.wantNum, tc.wantDen)
			}
			if got.Floor() != tc.wantFloor || got.Ceil() != tc.wantCeil {
				t.Fatalf("floor/ceil = %d/%d, want %d/%d", got.Floor(), got.Ceil(), tc.wantFloor, tc.wantCeil)
			}
		})
	}
}

func TestAnalyzeHalfFrame(t *testing.T) {
	exact, cands := Analyze(241)
	if exact != (Rat{Num: 241, Den: 2}) {
		t.Fatalf("exact = %s, want 241/2", exact)
	}
	if len(cands) != 2 {
		t.Fatalf("got %d candidates, want 2", len(cands))
	}
	want := []Candidate{
		{Tick: 120, Label: "floor", ErrorFine: -1},
		{Tick: 121, Label: "ceil", ErrorFine: +1},
	}
	for i, w := range want {
		if cands[i] != w {
			t.Fatalf("candidate %d = %+v, want %+v", i, cands[i], w)
		}
	}
	if e := CoarseErrorFine(241, 120); e != -1 {
		t.Fatalf("floor error = %d, want -1", e)
	}
	if e := CoarseErrorFine(241, 121); e != +1 {
		t.Fatalf("ceil error = %d, want +1", e)
	}
}

func TestAnalyzeExact(t *testing.T) {
	exact, cands := Analyze(240)
	if !exact.IsInteger() || exact.Num != 120 {
		t.Fatalf("exact = %s, want 120", exact)
	}
	if len(cands) != 1 || cands[0].Label != "exact" || cands[0].ErrorFine != 0 {
		t.Fatalf("candidates = %+v, want single exact", cands)
	}
}

func TestCumulativeError(t *testing.T) {
	ticks := []int64{240, 241, 242, 243, 244}
	// 奇数 tick 241、243 各贡献一次取整误差。
	if got := CumulativeErrorFine(ticks, FloorCoarse); got != -2 {
		t.Fatalf("floor cumulative = %d, want -2", got)
	}
	if got := CumulativeErrorFine(ticks, CeilCoarse); got != +2 {
		t.Fatalf("ceil cumulative = %d, want +2", got)
	}
	// 任何只输出整数粗帧 tick 的策略都无法对半帧做到零误差；
	// 无损的唯一方式是连同余数一起保留（见 TestLosslessRoundTripTenTimes）。
	for _, f := range ticks {
		stamp := Encode(f)
		if stamp.Decode() != f {
			t.Fatalf("Stamp(%d) 往返 = %d, want %d", f, stamp.Decode(), f)
		}
	}
}

func TestLosslessRoundTripTenTimes(t *testing.T) {
	for _, start := range []int64{0, 1, 240, 241, 11987, 11988} {
		fine := start
		for i := 0; i < 10; i++ {
			fine = RoundTrip(fine)
			if fine != start {
				t.Fatalf("start=%d after %d round trips = %d (snapped)", start, i+1, fine)
			}
		}
	}
}

func TestNaiveSnapsHalfFrame(t *testing.T) {
	// 半帧被 floor 吸附到较早的细帧，证明直接取整的做法不安全。
	if got := NaiveSnapFloor(241); got != 240 {
		t.Fatalf("naive floor = %d, want 240 (吸附演示)", got)
	}
	if got := NaiveSnapFloor(240); got != 240 {
		t.Fatalf("naive floor exact = %d, want 240", got)
	}
}

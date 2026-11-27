package handle

import "testing"

func TestPlanLossless(t *testing.T) {
	p, err := Plan(60_000, 10_000, 50_000, 2_000)
	if err != nil {
		t.Fatal(err)
	}
	if p.Head.Status != Lossless || p.Tail.Status != Lossless {
		t.Fatalf("want both lossless, got head=%v tail=%v", p.Head.Status, p.Tail.Status)
	}
	if p.Head.Available != 10_000 || p.Tail.Available != 10_000 {
		t.Fatalf("margins: head=%d tail=%d", p.Head.Available, p.Tail.Available)
	}
	if p.ExportStart != 8_000 || p.ExportEnd != 52_000 {
		t.Fatalf("export window [%d,%d)", p.ExportStart, p.ExportEnd)
	}
	if p.TotalDuration() != 44_000 {
		t.Fatalf("total %d", p.TotalDuration())
	}
	if p.NeedsReencode() {
		t.Fatal("unexpected reencode")
	}
}

func TestPlanFreeze(t *testing.T) {
	// 1s of head material, 2s handle required -> freeze 1s.
	p, err := Plan(60_000, 1_000, 50_000, 2_000)
	if err != nil {
		t.Fatal(err)
	}
	if p.Head.Status != Freeze {
		t.Fatalf("head status %v", p.Head.Status)
	}
	if p.Head.Deficit != 1_000 || p.Head.FreezeMs != 1_000 {
		t.Fatalf("head deficit=%d freeze=%d", p.Head.Deficit, p.Head.FreezeMs)
	}
	if p.ExportStart != 0 {
		t.Fatalf("export start clamped to 0, got %d", p.ExportStart)
	}
}

func TestPlanReencode(t *testing.T) {
	// Cut at the very edges: no material on either side.
	p, err := Plan(60_000, 0, 60_000, 2_000)
	if err != nil {
		t.Fatal(err)
	}
	if p.Head.Status != Reencode || p.Tail.Status != Reencode {
		t.Fatalf("head=%v tail=%v", p.Head.Status, p.Tail.Status)
	}
	if p.Head.FreezeMs != 2_000 || p.Tail.FreezeMs != 2_000 {
		t.Fatalf("freeze head=%d tail=%d", p.Head.FreezeMs, p.Tail.FreezeMs)
	}
	if !p.NeedsReencode() {
		t.Fatal("want reencode")
	}
}

func TestPlanInvalid(t *testing.T) {
	cases := [][4]int64{
		{0, 0, 1, 2_000},        // zero duration
		{60_000, -1, 10, 2_000}, // negative in
		{60_000, 10, 10, 2_000}, // empty range
		{60_000, 10, 61_000, 2_000},
		{60_000, 0, 10, -1}, // negative handle
	}
	for i, c := range cases {
		if _, err := Plan(c[0], c[1], c[2], c[3]); err == nil {
			t.Errorf("case %d: want error", i)
		}
	}
}

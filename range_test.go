package yuv

import "testing"

func TestLimitedRange10Bit(t *testing.T) {
	yLo, yHi, cLo, cHi, err := Limited.Bounds(10)
	if err != nil {
		t.Fatal(err)
	}
	if yLo != 64 || yHi != 940 || cLo != 64 || cHi != 960 {
		t.Fatalf("limited 10-bit bounds = %d..%d, %d..%d", yLo, yHi, cLo, cHi)
	}
}

func TestFullRange10Bit(t *testing.T) {
	yLo, yHi, cLo, cHi, err := Full.Bounds(10)
	if err != nil {
		t.Fatal(err)
	}
	if yLo != 0 || yHi != 1023 || cLo != 0 || cHi != 1023 {
		t.Fatalf("full 10-bit bounds = %d..%d, %d..%d", yLo, yHi, cLo, cHi)
	}
}

func TestRangeAliases(t *testing.T) {
	for _, s := range []string{"limited", "tv", "video"} {
		if got, err := ParseRange(s); err != nil || got != Limited {
			t.Fatalf("ParseRange(%q) = %q, %v", s, got, err)
		}
	}
	for _, s := range []string{"full", "pc", "jpeg"} {
		if got, err := ParseRange(s); err != nil || got != Full {
			t.Fatalf("ParseRange(%q) = %q, %v", s, got, err)
		}
	}
}

package budget

import "testing"

func TestParseAndFormat(t *testing.T) {
	cases := map[string]int64{
		"1024":  1024,
		"500GB": 500_000_000_000,
		"12tb":  12_000_000_000_000,
		"1.5MB": 1_500_000,
	}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("Parse(%q) = %d, want %d", in, got, want)
		}
	}
	if _, err := Parse("9XB"); err == nil {
		t.Fatal("expected error for unknown unit")
	}
	if got := Format(12_000_000_000_000); got != "12.00TB" {
		t.Fatalf("Format = %q", got)
	}
}

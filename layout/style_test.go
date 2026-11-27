package layout

import "testing"

func TestParseLengthUnits(t *testing.T) {
	cases := []struct {
		raw  string
		want float64
	}{
		{"32px", 32},
		{"50%", 40},
		{"2em", 64},
		{"2rem", 64},
		{"10rh", 72},
		{"10rw", 128},
	}
	for _, test := range cases {
		got, ok := parseLength(test.raw, 80, 32, 1280, 720)
		if !ok || !near(got, test.want) {
			t.Errorf("parseLength(%q) = %v, %v; want %v, true", test.raw, got, ok, test.want)
		}
	}
}

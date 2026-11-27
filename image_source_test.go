package ghostdiag

import "testing"

func TestNaturalLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"frame-2.png", "frame-10.png", true},
		{"frame-10.png", "frame-2.png", false},
		{"frame-02.png", "frame-2.png", false},
		{"a.png", "b.png", true},
	}
	for _, tt := range tests {
		if got := naturalLess(tt.a, tt.b); got != tt.want {
			t.Errorf("naturalLess(%q, %q)=%v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

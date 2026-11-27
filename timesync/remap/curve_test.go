package remap

import (
	"math"
	"testing"
)

func approx(t *testing.T, got, want, tol float64, name string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %g, want %g (tol %g)", name, got, want, tol)
	}
}

func TestConstantRateSegments(t *testing.T) {
	c, err := NewCurve([]Segment{
		{S0: 0, S1: 2, RateStart: 1, RateEnd: 1},
		{S0: 2, S1: 4, RateStart: 0.2, RateEnd: 0.2},
		{S0: 4, S1: 6, RateStart: 4, RateEnd: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	approx(t, c.Map(2), 2, 1e-12, "map end seg0")
	approx(t, c.Map(3), 7, 1e-12, "map inside 0.2x")
	approx(t, c.Map(5), 12.25, 1e-12, "map inside 4x")
	approx(t, c.OutputDuration(), 12.5, 1e-12, "output duration")

	for _, s := range []float64{0, 0.3, 2, 3.7, 4.1, 5.99, 6} {
		back := c.Inverse(c.Map(s))
		approx(t, back, s, 1e-9, "round-trip")
	}
	approx(t, c.Inverse(7), 3, 1e-12, "inverse seg boundary")
}

func TestLinearRateSegmentIntegral(t *testing.T) {
	c, err := NewCurve([]Segment{
		{S0: 0, S1: 2, RateStart: 1, RateEnd: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	// integral_0^2 1/(1+0.5u) du = 2 ln(2)
	approx(t, c.Map(2), 2*math.Log(2), 1e-12, "linear-rate integral")
	for _, s := range []float64{0.2, 1.0, 1.7, 2.0} {
		approx(t, c.Inverse(c.Map(s)), s, 1e-10, "linear-rate round-trip")
	}
}

func TestRateBoundariesAndGaps(t *testing.T) {
	cases := []struct {
		name string
		segs []Segment
	}{
		{"too slow", []Segment{{0, 1, 0.1, 0.1}}},
		{"too fast", []Segment{{0, 1, 5, 5}}},
		{"gap", []Segment{{0, 1, 1, 1}, {1.5, 2, 1, 1}}},
		{"zero duration", []Segment{{0, 0, 1, 1}}},
		{"not anchored", []Segment{{0.5, 1, 1, 1}}},
	}
	for _, tc := range cases {
		if _, err := NewCurve(tc.segs); err == nil {
			t.Fatalf("%s: expected validation error", tc.name)
		}
	}
}

func TestSamplePlanPerFrameMapping(t *testing.T) {
	c, _ := NewCurve([]Segment{
		{S0: 0, S1: 1, RateStart: 1, RateEnd: 1},
		{S0: 1, S1: 2, RateStart: 2, RateEnd: 2},
	})
	plan := SamplePlan{Curve: c, SourceRateHz: 1000, OutputRateHz: 1000, SourceFrameCount: 2000}
	if got := plan.OutputFrameCount(); got != 1500 {
		t.Fatalf("output frames = %d, want 1500", got)
	}
	// Frame 1250 lives at output t=1.25s, which is source s=1.5s.
	if got := plan.SourceFrameOf(1250); got != 1500 {
		t.Fatalf("source frame = %d, want 1500", got)
	}
}

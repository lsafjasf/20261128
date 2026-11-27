// Package remap defines the time-remapping model.
//
// A time-remapping curve describes, for every source time s, the output
// time t at which that source moment is played. The curve is the exact
// integral of a piecewise-linear playback-rate function r(s):
//
//	t(s) = integral_0^s 1/r(u) du
//
// Every mapping below is computed locally from the segment that contains
// the queried time. There is intentionally no global offset parameter:
// adding a constant to every mapped timestamp would corrupt the boundary
// conditions t(0)=0 and the per-segment continuity of the curve.
package remap

import (
	"errors"
	"fmt"
	"math"
)

// MinRate and MaxRate bound the accepted playback-rate range, matching the
// 0.2x..4x requirement.
const (
	MinRate = 0.2
	MaxRate = 4.0
	eps     = 1e-12
)

// Segment is one piece of a piecewise-linear speed curve.
//
// S0 and S1 are source-time boundaries in seconds. RateStart is the
// playback rate at S0 and RateEnd the rate at S1; the rate varies linearly
// inside the segment. A constant-rate segment sets both rates equal.
type Segment struct {
	S0, S1             float64
	RateStart, RateEnd float64
}

// Curve is a validated, continuous time-remapping curve.
type Curve struct {
	segments []Segment
}

// NewCurve validates the segments and builds a curve. Segments must be
// ordered, contiguous, start at source time zero, have positive duration,
// and every rate must lie within [MinRate, MaxRate].
func NewCurve(segs []Segment) (*Curve, error) {
	if len(segs) == 0 {
		return nil, errors.New("remap: curve needs at least one segment")
	}
	cp := make([]Segment, len(segs))
	var cursor float64
	for i, seg := range segs {
		if i == 0 {
			if math.Abs(seg.S0) > eps {
				return nil, fmt.Errorf("remap: first segment must start at 0, got %g", seg.S0)
			}
		} else if math.Abs(seg.S0-cursor) > eps {
			return nil, fmt.Errorf("remap: segment %d starts at %g, expected %g", i, seg.S0, cursor)
		}
		if seg.S1 <= seg.S0 {
			return nil, fmt.Errorf("remap: segment %d has non-positive duration", i)
		}
		if seg.RateStart < MinRate-eps || seg.RateStart > MaxRate+eps ||
			seg.RateEnd < MinRate-eps || seg.RateEnd > MaxRate+eps {
			return nil, fmt.Errorf("remap: segment %d rate outside [%g,%g]", i, MinRate, MaxRate)
		}
		if seg.RateStart <= 0 || seg.RateEnd <= 0 {
			return nil, fmt.Errorf("remap: segment %d has non-positive rate", i)
		}
		cp[i] = seg
		cursor = seg.S1
	}
	return &Curve{segments: cp}, nil
}

// Segments returns a copy of the curve segments.
func (c *Curve) Segments() []Segment {
	out := make([]Segment, len(c.segments))
	copy(out, c.segments)
	return out
}

// SourceDuration is the total source-time span in seconds.
func (c *Curve) SourceDuration() float64 { return c.segments[len(c.segments)-1].S1 }

// OutputDuration is t(SourceDuration), the remapped timeline length.
func (c *Curve) OutputDuration() float64 { return c.Map(c.SourceDuration()) }

// RateAt evaluates the piecewise-linear rate at source time s.
func (c *Curve) RateAt(s float64) float64 {
	seg, idx := c.segmentAtSource(s)
	if idx == len(c.segments)-1 && s >= seg.S1 {
		return seg.RateEnd
	}
	x := (s - seg.S0) / (seg.S1 - seg.S0)
	return seg.RateStart + x*(seg.RateEnd-seg.RateStart)
}

func (c *Curve) segmentAtSource(s float64) (Segment, int) {
	for i, seg := range c.segments {
		if s < seg.S1 || i == len(c.segments)-1 {
			return seg, i
		}
	}
	return c.segments[len(c.segments)-1], len(c.segments) - 1
}

func (c *Curve) segmentAtOutput(t float64) (Segment, int) {
	for i, seg := range c.segments {
		t1 := c.Map(seg.S1)
		if t < t1 || i == len(c.segments)-1 {
			return seg, i
		}
	}
	last := len(c.segments) - 1
	return c.segments[last], last
}

// Map converts a source time s to an output time t = integral 1/r.
func (c *Curve) Map(s float64) float64 {
	if s <= 0 {
		return 0
	}
	var t float64
	for _, seg := range c.segments {
		if s <= seg.S0 {
			break
		}
		u := math.Min(s, seg.S1)
		t += integrateInverseRate(seg, u-seg.S0)
		if s <= seg.S1 {
			break
		}
	}
	return t
}

// Inverse converts an output time t back to the source time s such that
// Map(s) = t. Constant segments invert directly; linear-rate segments
// solve the logarithmic/quadratic integral analytically.
func (c *Curve) Inverse(t float64) float64 {
	if t <= 0 {
		return 0
	}
	var s float64
	for _, seg := range c.segments {
		span := integrateInverseRate(seg, seg.S1-seg.S0)
		if t > span && s+span < c.OutputDuration()-eps {
			s = seg.S1
			t -= span
			continue
		}
		d := inverseIntegral(seg, t)
		return seg.S0 + d
	}
	return c.SourceDuration()
}

// integrateInverseRate returns integral_0^d 1/(a+b*u) du where a and b are
// the segment rates at its start and the rate slope over its duration.
func integrateInverseRate(seg Segment, d float64) float64 {
	a := seg.RateStart
	if math.Abs(seg.RateEnd-seg.RateStart) < eps {
		return d / a
	}
	b := (seg.RateEnd - seg.RateStart) / (seg.S1 - seg.S0)
	return math.Log((a+b*d)/a) / b
}

// inverseIntegral solves t = integral_0^d 1/r(u) du for d.
func inverseIntegral(seg Segment, t float64) float64 {
	a := seg.RateStart
	if math.Abs(seg.RateEnd-seg.RateStart) < eps {
		return t * a
	}
	b := (seg.RateEnd - seg.RateStart) / (seg.S1 - seg.S0)
	return (a*math.Exp(b*t) - a) / b
}

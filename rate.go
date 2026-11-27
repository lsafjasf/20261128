package subburn

import (
	"fmt"
	"math/bits"
	"strconv"
	"strings"
	"time"
)

// Rate is an exact frame rate expressed as Num/Den frames per second.
// NTSC rates must be represented as 30000/1001, not the rounded 29.97.
type Rate struct {
	Num int64
	Den int64
}

func NewRate(num, den int64) (Rate, error) {
	if num <= 0 || den <= 0 {
		return Rate{}, fmt.Errorf("frame rate must be positive: %d/%d", num, den)
	}
	g := gcd(num, den)
	return Rate{Num: num / g, Den: den / g}, nil
}

// ParseRate accepts either a fraction ("30000/1001") or an exact decimal
// ("25", "29.97"). A decimal is not silently promoted to an NTSC rational.
func ParseRate(value string) (Rate, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Rate{}, fmt.Errorf("empty frame rate")
	}
	if strings.Contains(value, "/") {
		parts := strings.Split(value, "/")
		if len(parts) != 2 {
			return Rate{}, fmt.Errorf("invalid frame rate %q", value)
		}
		num, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			return Rate{}, fmt.Errorf("invalid frame rate %q: %w", value, err)
		}
		den, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return Rate{}, fmt.Errorf("invalid frame rate %q: %w", value, err)
		}
		return NewRate(num, den)
	}

	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return Rate{}, fmt.Errorf("invalid frame rate %q", value)
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Rate{}, fmt.Errorf("invalid frame rate %q: %w", value, err)
	}
	if len(parts) == 1 {
		return NewRate(whole, 1)
	}
	if parts[1] == "" {
		return Rate{}, fmt.Errorf("invalid frame rate %q", value)
	}
	frac, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Rate{}, fmt.Errorf("invalid frame rate %q: %w", value, err)
	}
	scale := int64(1)
	for range parts[1] {
		if scale > (1<<62)/10 {
			return Rate{}, fmt.Errorf("frame rate %q has too many decimal places", value)
		}
		scale *= 10
	}
	return NewRate(whole*scale+frac, scale)
}

func (r Rate) Valid() bool {
	return r.Num > 0 && r.Den > 0 &&
		uint64(r.Den) <= uint64(^uint64(0)>>1)/uint64(time.Second)
}

func (r Rate) Equal(other Rate) bool {
	return r.Num == other.Num && r.Den == other.Den
}

func (r Rate) String() string {
	if !r.Valid() {
		return "invalid"
	}
	if r.Den == 1 {
		return strconv.FormatInt(r.Num, 10)
	}
	return fmt.Sprintf("%d/%d", r.Num, r.Den)
}

// FramePTS returns the exact presentation time of a frame, truncated to one
// nanosecond. It never adds a per-frame duration to a previous value, so error
// cannot accumulate from frame to frame.
func (r Rate) FramePTS(frame int64) time.Duration {
	if !r.Valid() || frame < 0 {
		return 0
	}
	ns := mulDivFloor(uint64(frame), uint64(r.Den)*uint64(time.Second), uint64(r.Num))
	if ns > uint64(^uint64(0)>>1) {
		return time.Duration(^uint64(0) >> 1)
	}
	return time.Duration(ns)
}

// FrameAt returns the index of the frame whose half-open display interval
// [FramePTS(i), FramePTS(i+1)) contains pts.
func (r Rate) FrameAt(pts time.Duration) int64 {
	if !r.Valid() || pts <= 0 {
		return 0
	}
	frame := int64(mulDivFloor(uint64(pts), uint64(r.Num), uint64(r.Den)*uint64(time.Second)))
	for frame > 0 && r.FramePTS(frame) > pts {
		frame--
	}
	for r.FramePTS(frame+1) <= pts {
		frame++
	}
	return frame
}

// FrameAtOrAfter returns the first frame whose PTS is not earlier than pts.
// Subtitle ranges use [Start, End), so both ends are mapped with this rule.
func (r Rate) FrameAtOrAfter(pts time.Duration) int64 {
	if !r.Valid() || pts <= 0 {
		return 0
	}
	frame := r.FrameAt(pts)
	if r.FramePTS(frame) < pts {
		frame++
	}
	return frame
}

// FrameCount returns the number of frame intervals needed to cover duration.
// It is ceil(duration * rate), because a final partial frame is still displayed.
func (r Rate) FrameCount(duration time.Duration) int64 {
	if !r.Valid() || duration <= 0 {
		return 0
	}
	den := uint64(r.Den) * uint64(time.Second)
	count, remainder := mulDivRem(uint64(duration), uint64(r.Num), den)
	if remainder != 0 {
		count++
	}
	return int64(count)
}

func (r Rate) FrameDuration() time.Duration {
	if !r.Valid() {
		return 0
	}
	v := mulDivFloor(uint64(r.Den)*uint64(time.Second), 1, uint64(r.Num))
	if v > uint64(^uint64(0)>>1) {
		return time.Duration(^uint64(0) >> 1)
	}
	return time.Duration(v)
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// mulDivFloor computes floor(a*b/c) without overflowing the intermediate
// product. All inputs are non-negative and c must be non-zero.
func mulDivFloor(a, b, c uint64) uint64 {
	q, _ := mulDivRem(a, b, c)
	return q
}

func mulDivRem(a, b, c uint64) (uint64, uint64) {
	hi, lo := bits.Mul64(a, b)
	if hi >= c {
		panic("mulDivFloor overflow")
	}
	return bits.Div64(hi, lo, c)
}

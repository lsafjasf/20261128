// Package lc3 parses re-packetized LC3 audio and reconstructs an exact
// PCM-sample timeline, including the gaps left by lost packets.
//
// LC3 supports two frame durations, 10 ms and 7.5 ms. A player that assumes
// every frame occupies a fixed duration drifts as soon as the encoder switches
// duration or emits a short tail frame. The helpers in this package always
// derive the duration of each frame from its own framing metadata instead of
// relying on a global constant.
package lc3

import (
	"errors"
	"fmt"
)

// Duration selects one of the two legal LC3 frame durations.
type Duration uint8

const (
	// Dur10ms is the 10 millisecond LC3 frame duration.
	Dur10ms Duration = 10
	// Dur7p5ms is the 7.5 millisecond LC3 frame duration.
	Dur7p5ms Duration = 7
)

// Milliseconds returns the frame duration as a fractional number of
// milliseconds. 7.5 ms cannot be represented as an integer, so callers that
// need exact arithmetic should use Samples and the clock rate.
func (d Duration) Milliseconds() float64 {
	switch d {
	case Dur10ms:
		return 10
	case Dur7p5ms:
		return 7.5
	default:
		return 0
	}
}

// Valid reports whether d is one of the two LC3 frame durations.
func (d Duration) Valid() bool {
	return d == Dur10ms || d == Dur7p5ms
}

// Microseconds returns the frame duration in whole microseconds. Both legal
// durations are exact on this scale (10000 us and 7500 us), so no rounding is
// involved.
func (d Duration) Microseconds() int64 {
	switch d {
	case Dur10ms:
		return 10_000
	case Dur7p5ms:
		return 7_500
	default:
		return 0
	}
}

// frameSamples maps sample rate and frame duration to the number of PCM
// samples produced by an LC3 decoder for one frame (the Bluetooth LC3 "Ns"
// table). The zero value means the combination is unsupported.
var frameSamples = map[uint32]map[Duration]int{
	8000:  {Dur10ms: 80, Dur7p5ms: 60},
	16000: {Dur10ms: 160, Dur7p5ms: 120},
	24000: {Dur10ms: 240, Dur7p5ms: 180},
	32000: {Dur10ms: 320, Dur7p5ms: 240},
	48000: {Dur10ms: 480, Dur7p5ms: 360},
}

// SupportedRate reports whether rate is one of the five LC3 sample rates.
func SupportedRate(rate uint32) bool {
	_, ok := frameSamples[rate]
	return ok
}

// Samples returns the exact number of PCM samples occupied by one LC3 frame at
// the given sample rate. It returns an error for an unknown rate or duration.
func Samples(rate uint32, d Duration) (int, error) {
	durs, ok := frameSamples[rate]
	if !ok {
		return 0, fmt.Errorf("lc3: unsupported sample rate %d Hz", rate)
	}
	n, ok := durs[d]
	if !ok || !d.Valid() {
		return 0, fmt.Errorf("lc3: invalid frame duration %d", d)
	}
	return n, nil
}

// ErrShortBuffer means a frame or header was truncated relative to the length
// declared in its own framing metadata.
var ErrShortBuffer = errors.New("lc3: truncated frame buffer")

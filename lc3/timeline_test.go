package lc3

import "testing"

func frame(seq uint16, dur Duration, octets int) Frame {
	return Frame{Seq: seq, Duration: dur, Octets: octets, Payload: make([]byte, octets)}
}

// TestModeSwitchTimeline verifies that a stream switching from 10 ms to 7.5 ms
// frames is laid out using each frame's own sample count rather than a fixed
// duration.
func TestModeSwitchTimeline(t *testing.T) {
	const rate = uint32(48000)
	frames := []Frame{
		frame(0, Dur10ms, 120), // 480 samples
		frame(1, Dur10ms, 120), // 480 samples
		frame(2, Dur7p5ms, 90), // 360 samples
		frame(3, Dur7p5ms, 90), // 360 samples
		frame(4, Dur10ms, 110), // 480 samples
	}
	tl, err := Build(rate, frames)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	wantStarts := []uint64{0, 480, 960, 1320, 1680}
	wantEnds := []uint64{480, 960, 1320, 1680, 2160}
	for i, seg := range tl.Frames() {
		if seg.Start != wantStarts[i] || seg.End != wantEnds[i] {
			t.Errorf("frame %d interval [%d,%d), want [%d,%d)", i, seg.Start, seg.End, wantStarts[i], wantEnds[i])
		}
	}
	if tl.TotalSamples() != 2160 {
		t.Fatalf("total = %d, want 2160", tl.TotalSamples())
	}

	// A naive fixed-10ms player would charge 480 samples to every frame and
	// report 2400 samples, overstating the timeline by 240 samples (one 7.5ms
	// frame worth of drift for the two short frames).
	naive := uint64(len(frames)) * 480
	if naive != 2400 || tl.TotalSamples() >= naive {
		t.Fatalf("fixed-duration drift not demonstrated: naive=%d exact=%d", naive, tl.TotalSamples())
	}
}

// TestLossGaps confirms sequence-number holes become gap segments whose sample
// span and missing-frame count are exact, even across a mode switch.
func TestLossGaps(t *testing.T) {
	const rate = uint32(32000)
	frames := []Frame{
		frame(10, Dur10ms, 80),
		frame(11, Dur10ms, 80),
		// seqs 12,13 lost
		frame(14, Dur7p5ms, 60),
		// seq 15 lost
		frame(16, Dur7p5ms, 60),
	}
	tl, err := Build(rate, frames)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	gaps := tl.Gaps()
	if len(gaps) != 2 {
		t.Fatalf("got %d gaps, want 2: %+v", len(gaps), gaps)
	}
	// First gap: 2 missing frames. At 32kHz a frame is 320 (10ms) / 240 (7.5ms).
	// The gap is estimated with the resuming frame's duration (7.5ms -> 240).
	g0 := gaps[0]
	if g0.Seq != 12 || g0.Lost != 2 {
		t.Errorf("gap0 seq=%d lost=%d, want seq 12 lost 2", g0.Seq, g0.Lost)
	}
	if g0.Length() != uint64(2*240) {
		t.Errorf("gap0 length=%d, want %d", g0.Length(), 2*240)
	}
	// Gap0 starts right after frame 11 (two 10ms frames = 640 samples).
	if g0.Start != 640 {
		t.Errorf("gap0 start=%d, want 640", g0.Start)
	}
	g1 := gaps[1]
	if g1.Seq != 15 || g1.Lost != 1 || g1.Length() != 240 {
		t.Errorf("gap1 = seq %d lost %d len %d, want 15/1/240", g1.Seq, g1.Lost, g1.Length())
	}
	if tl.Received != 4 || tl.Lost != 3 {
		t.Errorf("counts received=%d lost=%d, want 4/3", tl.Received, tl.Lost)
	}

	// Segments must form a contiguous, non-overlapping timeline.
	var prev uint64
	for i, seg := range tl.Segments {
		if seg.Start != prev {
			t.Fatalf("segment %d starts at %d, expected %d (discontinuity)", i, seg.Start, prev)
		}
		prev = seg.End
	}
}

// TestSequenceRollover checks 16-bit wrap: after 65535 the next frame is 0.
func TestSequenceRollover(t *testing.T) {
	frames := []Frame{
		frame(65534, Dur10ms, 40),
		frame(65535, Dur10ms, 40),
		frame(0, Dur10ms, 40),
		// one frame (seq 1) lost
		frame(2, Dur10ms, 40),
	}
	tl, err := Build(16000, frames)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	gaps := tl.Gaps()
	if len(gaps) != 1 || gaps[0].Seq != 1 || gaps[0].Lost != 1 {
		t.Fatalf("rollover gap wrong: %+v", gaps)
	}
}

func TestRejectOutOfOrder(t *testing.T) {
	frames := []Frame{frame(5, Dur10ms, 10), frame(4, Dur10ms, 10)}
	if _, err := Build(48000, frames); err == nil {
		t.Fatal("expected error for backward sequence number")
	}
}

func TestFixedGapPolicy(t *testing.T) {
	const rate = uint32(48000)
	// A 10ms frame, two lost frames, then a 7.5ms resuming frame.
	frames := []Frame{
		frame(0, Dur10ms, 100),
		frame(3, Dur7p5ms, 80),
	}
	tl, err := BuildWithOptions(rate, frames, Options{
		Gap:              GapUseFixedDuration,
		FixedGapDuration: Dur10ms,
	})
	if err != nil {
		t.Fatalf("BuildWithOptions: %v", err)
	}
	gap := tl.Gaps()[0]
	if gap.Lost != 2 || gap.Duration != Dur10ms || gap.Length() != 2*480 {
		t.Fatalf("fixed gap wrong: %+v", gap)
	}

	if _, err := BuildWithOptions(rate, frames, Options{Gap: GapUseFixedDuration}); err == nil {
		t.Fatal("expected error when fixed policy has no valid duration")
	}
}

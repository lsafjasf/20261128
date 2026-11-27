package amrwb

import (
	"errors"
	"math"
	"testing"
)

func TestModeFrameSizes(t *testing.T) {
	tests := []struct {
		mode    Mode
		size    int
		payload int
	}{
		{Mode660, 18, 17},
		{Mode885, 24, 23},
		{Mode1265, 33, 32},
		{Mode1425, 37, 36},
		{Mode1585, 41, 40},
		{Mode1825, 47, 46},
		{Mode1985, 51, 50},
		{Mode2305, 59, 58},
		{Mode2385, 61, 60},
		{ModeSID, 6, 5},
		{ModeSpeechLost, 1, 0},
		{ModeNoData, 1, 0},
	}
	for _, tt := range tests {
		if got := tt.mode.FrameOctets(); got != tt.size {
			t.Errorf("%s frame size = %d, want %d", tt.mode, got, tt.size)
		}
		if got := tt.mode.PayloadOctets(); got != tt.payload {
			t.Errorf("%s payload size = %d, want %d", tt.mode, got, tt.payload)
		}
	}
	if SamplesPerFrame != 320 {
		t.Fatalf("SamplesPerFrame = %d, want 320", SamplesPerFrame)
	}
}

func TestParseHeader(t *testing.T) {
	tests := []struct {
		octet   byte
		mode    Mode
		quality FrameQuality
	}{
		{0x04, Mode660, QualityGood},
		{0x14, Mode1265, QualityGood},
		{0x40, Mode2385, QualityBad},
		{0x74, ModeSpeechLost, QualityGood},
		{0x7c, ModeNoData, QualityGood},
	}
	for _, tt := range tests {
		h := ParseHeader(tt.octet)
		if h.Mode != tt.mode || h.Quality != tt.quality {
			t.Errorf("ParseHeader(%#x) = {%d, %d}, want {%d, %d}",
				tt.octet, h.Mode, h.Quality, tt.mode, tt.quality)
		}
	}
}

func TestFrequentModeSwitchDoesNotCreateLoss(t *testing.T) {
	modes := []Mode{
		Mode660, Mode885, Mode1265, Mode1425, Mode1585,
		Mode1825, Mode1985, Mode2305, Mode2385,
		Mode2305, Mode1985, Mode1825, Mode1585, Mode1425,
		Mode1265, Mode885, Mode660, Mode2385,
	}
	frames := make([]Frame, 0, len(modes))
	for i, mode := range modes {
		frames = append(frames, Frame{
			Index:   int64(i),
			Mode:    mode,
			Size:    mode.FrameOctets(),
			Quality: QualityGood,
		})
	}
	timeline, err := BuildTimeline(frames)
	if err != nil {
		t.Fatalf("BuildTimeline: %v", err)
	}

	stats := timeline.Stats()
	wantSlots := len(modes)
	if stats.FrameSlots != wantSlots {
		t.Errorf("FrameSlots = %d, want %d", stats.FrameSlots, wantSlots)
	}
	if stats.Samples != uint64(wantSlots*SamplesPerFrame) {
		t.Errorf("Samples = %d, want %d", stats.Samples, wantSlots*SamplesPerFrame)
	}
	if stats.LostFrames != 0 || stats.ConcealedFrames != 0 {
		t.Errorf("mode changes were treated as loss: %+v", stats)
	}
	if stats.ModeChanges != 17 {
		t.Errorf("ModeChanges = %d, want 17", stats.ModeChanges)
	}
	if !stats.Consistent() || stats.HeaderSamples() != stats.Samples {
		t.Errorf("inconsistent totals: %+v", stats)
	}

	spans := timeline.Spans()
	for i, span := range spans {
		wantStart := uint64(i * SamplesPerFrame)
		if span.Index != int64(i) || span.SampleStart != wantStart ||
			span.SampleEnd != wantStart+SamplesPerFrame || span.Samples != SamplesPerFrame {
			t.Fatalf("span %d = %+v, want contiguous %d..%d",
				i, span, wantStart, wantStart+SamplesPerFrame)
		}
	}
}

func TestBadFrameKeepsFollowingIndicesAligned(t *testing.T) {
	timeline := NewTimeline()
	frames := []Frame{
		{Index: 0, Mode: Mode660, Size: 18, Quality: QualityGood},
		{Index: 1, Mode: Mode1265, Size: 33, Quality: QualityBad},
		{Index: 2, Mode: Mode2385, Size: 61, Quality: QualityGood},
	}
	if err := timeline.AddAll(frames); err != nil {
		t.Fatalf("AddAll: %v", err)
	}

	spans := timeline.Spans()
	if len(spans) != 3 {
		t.Fatalf("got %d spans, want 3", len(spans))
	}
	if spans[1].Disposition != DispositionBad || !spans[1].Concealed {
		t.Errorf("bad frame disposition = %s, concealed = %v",
			spans[1].Disposition, spans[1].Concealed)
	}
	if spans[1].SampleStart != 320 || spans[1].SampleEnd != 640 {
		t.Errorf("bad frame span = %d..%d, want 320..640",
			spans[1].SampleStart, spans[1].SampleEnd)
	}
	if spans[2].Index != 2 || spans[2].SampleStart != 640 || spans[2].SampleEnd != 960 {
		t.Errorf("frame after bad frame = %+v, want index 2 at 640..960", spans[2])
	}

	stats := timeline.Stats()
	if stats.Samples != 960 || stats.BadFrames != 1 || stats.ConcealedFrames != 1 {
		t.Errorf("stats = %+v, want 960 samples, one bad and one concealed", stats)
	}
	if !stats.Consistent() {
		t.Errorf("inconsistent totals after bad frame: %+v", stats)
	}
	if stats.LostFrames != 0 {
		t.Errorf("bad frame counted as lost: %+v", stats)
	}
}

func TestIndexGapCreatesFixedConcealment(t *testing.T) {
	timeline := NewTimeline()
	if _, err := timeline.Add(Frame{Index: 0, Mode: Mode660, Size: 18}); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	span, err := timeline.Add(Frame{Index: 4, Mode: Mode885, Size: 24})
	if err != nil {
		t.Fatalf("second Add: %v", err)
	}
	if span.SampleStart != 4*SamplesPerFrame || span.SampleEnd != 5*SamplesPerFrame {
		t.Errorf("frame 4 span = %d..%d, want 1280..1600", span.SampleStart, span.SampleEnd)
	}

	stats := timeline.Stats()
	if stats.FrameSlots != 5 || stats.Samples != 1600 {
		t.Errorf("stats = %+v, want five slots and 1600 samples", stats)
	}
	if stats.LostFrames != 3 || stats.ConcealedFrames != 3 || stats.GoodFrames != 2 {
		t.Errorf("stats = %+v, want three lost and two good frames", stats)
	}

	spans := timeline.Spans()
	for i := 1; i <= 3; i++ {
		if spans[i].Disposition != DispositionLost || !spans[i].Concealed ||
			spans[i].Samples != SamplesPerFrame {
			t.Errorf("gap span %d = %+v", i, spans[i])
		}
	}
}

func TestExplicitLossAndNoDataAreDistinct(t *testing.T) {
	timeline := NewTimeline()
	frames := []Frame{
		{Index: 0, Mode: Mode660, Size: 18},
		{Index: 1, Mode: ModeSpeechLost, Size: 1},
		{Index: 2, Mode: ModeNoData, Size: 1},
		{Index: 3, Mode: Mode885, Size: 24},
	}
	if err := timeline.AddAll(frames); err != nil {
		t.Fatalf("AddAll: %v", err)
	}

	stats := timeline.Stats()
	if stats.LostFrames != 1 || stats.NoDataFrames != 1 || stats.ConcealedFrames != 2 {
		t.Errorf("stats = %+v, want one lost and one no-data slot", stats)
	}
	if stats.Samples != 4*SamplesPerFrame {
		t.Errorf("Samples = %d, want %d", stats.Samples, 4*SamplesPerFrame)
	}
	last := timeline.Spans()[3]
	if last.SampleStart != 3*SamplesPerFrame || last.SampleEnd != 4*SamplesPerFrame {
		t.Errorf("last span = %+v, want 960..1280", last)
	}
}

func TestInvalidSizeDoesNotMutateTimeline(t *testing.T) {
	timeline := NewTimeline()
	if _, err := timeline.Add(Frame{Index: 0, Mode: Mode660, Size: 18}); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	before := timeline.Stats()

	_, err := timeline.Add(Frame{Index: 1, Mode: Mode660, Size: 24})
	if !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("Add error = %v, want ErrInvalidFrame", err)
	}
	if got := timeline.Stats(); got != before {
		t.Fatalf("failed Add changed stats: before=%+v after=%+v", before, got)
	}

	span, err := timeline.Add(Frame{Index: 1, Mode: Mode885, Size: 24})
	if err != nil {
		t.Fatalf("Add after rejected frame: %v", err)
	}
	if span.SampleStart != 320 || span.Index != 1 {
		t.Errorf("span after rejected frame = %+v, want index 1 at 320", span)
	}
}

func TestOutOfOrderAndOverflow(t *testing.T) {
	timeline := NewTimeline()
	if _, err := timeline.Add(Frame{Index: 0, Mode: Mode660, Size: 18}); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if _, err := timeline.Add(Frame{Index: 0, Mode: Mode660, Size: 18}); !errors.Is(err, ErrOutOfOrder) {
		t.Fatalf("duplicate Add error = %v, want ErrOutOfOrder", err)
	}
	if _, err := timeline.Add(Frame{
		Index: math.MaxInt64, Mode: Mode660, Size: 18,
	}); !errors.Is(err, ErrTimelineOverflow) {
		t.Fatalf("huge gap Add error = %v, want ErrTimelineOverflow", err)
	}
}

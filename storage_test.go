package amrwb

import (
	"errors"
	"testing"
)

func appendStorageFrame(dst []byte, mode Mode, bad bool) []byte {
	header := byte(mode) << 3
	if !bad {
		header |= 0x04
	}
	dst = append(dst, header)
	payload := mode.FrameOctets() - 1
	for i := 0; i < payload; i++ {
		dst = append(dst, byte(i))
	}
	return dst
}

func TestParseStorageTimeline(t *testing.T) {
	data := []byte(StorageMagic)
	data = appendStorageFrame(data, Mode660, false)
	data = appendStorageFrame(data, Mode2385, true)
	data = appendStorageFrame(data, Mode1265, false)
	data = appendStorageFrame(data, ModeSpeechLost, false)
	data = appendStorageFrame(data, ModeNoData, false)
	data = appendStorageFrame(data, Mode885, false)

	timeline, err := ParseStorageBytes(data)
	if err != nil {
		t.Fatalf("ParseStorageBytes: %v", err)
	}

	stats := timeline.Stats()
	if stats.FrameSlots != 6 || stats.Samples != 6*SamplesPerFrame {
		t.Errorf("stats = %+v, want six slots and 1920 samples", stats)
	}
	if stats.GoodFrames != 3 || stats.BadFrames != 1 ||
		stats.LostFrames != 1 || stats.NoDataFrames != 1 {
		t.Errorf("stats = %+v, want 3 good, 1 bad, 1 lost, 1 no-data", stats)
	}
	if stats.ConcealedFrames != 3 {
		t.Errorf("ConcealedFrames = %d, want 3", stats.ConcealedFrames)
	}
	if stats.ModeChanges != 3 {
		t.Errorf("ModeChanges = %d, want 3", stats.ModeChanges)
	}
	if !stats.Consistent() {
		t.Errorf("inconsistent storage totals: %+v", stats)
	}

	spans := timeline.Spans()
	for i, span := range spans {
		wantStart := uint64(i * SamplesPerFrame)
		if span.SampleStart != wantStart || span.SampleEnd != wantStart+SamplesPerFrame {
			t.Errorf("span %d = %+v, want contiguous %d..%d",
				i, span, wantStart, wantStart+SamplesPerFrame)
		}
	}
	if spans[1].Disposition != DispositionBad {
		t.Errorf("span 1 disposition = %s, want bad", spans[1].Disposition)
	}
	if spans[3].Disposition != DispositionLost || spans[4].Disposition != DispositionNoData {
		t.Errorf("FT=14/15 dispositions = %s/%s, want lost/no_data",
			spans[3].Disposition, spans[4].Disposition)
	}
	if spans[5].SampleStart != 5*SamplesPerFrame {
		t.Errorf("span after bad/lost/no-data starts at %d, want %d",
			spans[5].SampleStart, 5*SamplesPerFrame)
	}
}

func TestParseStorageEmptyFile(t *testing.T) {
	timeline, err := ParseStorageBytes([]byte(StorageMagic))
	if err != nil {
		t.Fatalf("ParseStorageBytes: %v", err)
	}
	if got := timeline.Stats(); got != (Stats{}) {
		t.Errorf("empty storage stats = %+v, want zero", got)
	}
}

func TestParseStorageTruncatedFrame(t *testing.T) {
	data := []byte(StorageMagic)
	data = append(data, byte(Mode885)<<3|0x04)
	data = append(data, make([]byte, 8)...)

	_, err := ParseStorageBytes(data)
	if !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("ParseStorageBytes error = %v, want ErrInvalidFrame", err)
	}
}

func TestParseStorageRejectsInvalidHeader(t *testing.T) {
	tests := [][]byte{
		[]byte("not an AMR-WB file"),
		append([]byte(StorageMagic), 0x54),
	}
	for _, data := range tests {
		_, err := ParseStorageBytes(data)
		if !errors.Is(err, ErrInvalidFrame) {
			t.Errorf("ParseStorageBytes(%q) error = %v, want ErrInvalidFrame",
				data, err)
		}
	}
}

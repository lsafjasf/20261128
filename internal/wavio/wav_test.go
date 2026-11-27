package wavio

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadPCM16Mono(t *testing.T) {
	path := filepath.Join(t.TempDir(), "probe.wav")
	want := []float64{-0.5, -0.1, 0, 0.25, 0.75}
	if err := WritePCM16Mono(path, want, 48000); err != nil {
		t.Fatal(err)
	}
	got, info, err := ReadMono(path, -1)
	if err != nil {
		t.Fatal(err)
	}
	if info.SampleRate != 48000 || info.Channels != 1 || info.Bits != 16 {
		t.Fatalf("info=%+v", info)
	}
	if len(got) != len(want) {
		t.Fatalf("length %d", len(got))
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1.0/32767 {
			t.Fatalf("sample %d got %.6f want %.6f", i, got[i], want[i])
		}
	}
}

func TestDecodePCM24SignExtension(t *testing.T) {
	pos := decodeSample([]byte{0x00, 0x00, 0x40}, formatPCM, 24)
	neg := decodeSample([]byte{0x00, 0x00, 0xc0}, formatPCM, 24)
	if math.Abs(pos-0.5) > 1e-9 || math.Abs(neg+0.5) > 1e-9 {
		t.Fatalf("24-bit decode positive=%f negative=%f", pos, neg)
	}
}

func TestReadRejectsNonWave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.wav")
	if err := os.WriteFile(path, []byte("not a wave file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadMono(path, -1); err == nil {
		t.Fatal("expected error")
	}
}

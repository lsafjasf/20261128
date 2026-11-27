package lfcheck

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestParseWAVPCM16Channels(t *testing.T) {
	left := []float64{0.25, -0.5, 0.75}
	right := []float64{-0.25, 0.5, -0.75}
	data := pcm16WAV(2, 48000, [][]float64{left, right})
	audio, err := ParseWAV(data)
	if err != nil {
		t.Fatal(err)
	}
	if audio.Channels != 2 || audio.SampleRate != 48000 || audio.Bits != 16 {
		t.Fatalf("unexpected format: %+v", audio)
	}
	if got := len(audio.Samples[0]); got != 3 {
		t.Fatalf("frames = %d, want 3", got)
	}
	for frame, want := range left {
		if got := audio.Samples[0][frame]; math.Abs(got-want) > 1.0/32767 {
			t.Errorf("left[%d] = %.6f, want %.6f", frame, got, want)
		}
	}
}

func TestParseWAVExtensibleMask(t *testing.T) {
	data := writeExtensibleWAV(6, 48000, 0x3f)
	audio, err := ParseWAV(data)
	if err != nil {
		t.Fatal(err)
	}
	if !audio.HasMask || audio.ChannelMask != 0x3f {
		t.Fatalf("mask not retained: has=%t mask=0x%x", audio.HasMask, audio.ChannelMask)
	}
	names, err := ChannelNames(audio)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"FL", "FR", "FC", "LFE", "BL", "BR"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("layout = %v, want %v", names, want)
		}
	}
}

func TestRolesFromLayoutAliases(t *testing.T) {
	roles, err := RolesFromLayout([]string{"LEFT", "RIGHT", "CENTER", "SUB", "Ls", "Rs"})
	if err != nil {
		t.Fatal(err)
	}
	if roles.Left != 0 || roles.Right != 1 || roles.Center != 2 || roles.LFE != 3 {
		t.Fatalf("unexpected roles: %+v", roles)
	}
}

func writeExtensibleWAV(channels, sampleRate int, mask uint32) []byte {
	frames := 1
	blockAlign := channels * 2
	fmtSize := 40
	dataSize := frames * blockAlign
	size := 12 + 8 + fmtSize + 8 + dataSize
	out := make([]byte, size)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(size-8))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], uint32(fmtSize))
	binary.LittleEndian.PutUint16(out[20:22], 0xfffe)
	binary.LittleEndian.PutUint16(out[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(sampleRate*blockAlign))
	binary.LittleEndian.PutUint16(out[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[34:36], 16)
	binary.LittleEndian.PutUint16(out[36:38], 22)
	binary.LittleEndian.PutUint16(out[38:40], 16)
	binary.LittleEndian.PutUint32(out[40:44], mask)
	binary.LittleEndian.PutUint16(out[44:46], 1)
	copy(out[46:56], []byte{0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71})
	copy(out[60:64], "data")
	binary.LittleEndian.PutUint32(out[64:68], uint32(dataSize))
	return out
}

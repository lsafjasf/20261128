package channelmap

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func makeExtensibleWAV(channels uint16, mask uint32) []byte {
	const fmtSize = 40
	var data bytes.Buffer
	data.Write([]byte("RIFF"))
	data.Write(make([]byte, 4))
	data.Write([]byte("WAVE"))

	data.Write([]byte("fmt "))
	_ = binary.Write(&data, binary.LittleEndian, uint32(fmtSize))
	_ = binary.Write(&data, binary.LittleEndian, uint16(0xFFFE))
	_ = binary.Write(&data, binary.LittleEndian, channels)
	_ = binary.Write(&data, binary.LittleEndian, uint32(48000))
	_ = binary.Write(&data, binary.LittleEndian, uint32(48000*channels*2))
	_ = binary.Write(&data, binary.LittleEndian, uint16(channels*2))
	_ = binary.Write(&data, binary.LittleEndian, uint16(16))
	_ = binary.Write(&data, binary.LittleEndian, uint16(22))
	_ = binary.Write(&data, binary.LittleEndian, uint16(16))
	_ = binary.Write(&data, binary.LittleEndian, mask)
	_ = binary.Write(&data, binary.LittleEndian, uint32(1))
	data.Write([]byte{0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xAA,
		0x00, 0x38, 0x9B, 0x71})

	data.Write([]byte("data"))
	_ = binary.Write(&data, binary.LittleEndian, uint32(0))
	binary.LittleEndian.PutUint32(data.Bytes()[4:8], uint32(data.Len()-8))
	return data.Bytes()
}

func TestParseWAVExtensibleChannelMask(t *testing.T) {
	layout, err := ParseContainer(makeExtensibleWAV(6, 0x0000003F), "test.wav")
	if err != nil {
		t.Fatalf("ParseContainer() error = %v", err)
	}
	if layout.Container != "WAV" || layout.Codec != "PCM" {
		t.Fatalf("unexpected container metadata: %+v", layout)
	}
	if layout.SampleRate != 48000 {
		t.Fatalf("sample rate = %d, want 48000", layout.SampleRate)
	}
	if layout.ChannelMask == nil || *layout.ChannelMask != 0x3F {
		t.Fatalf("channel mask = %v, want 0x3F", layout.ChannelMask)
	}
	want := []Label{LabelLeft, LabelRight, LabelCenter, LabelLFE,
		LabelLeftSurround, LabelRightSurround}
	assertLabels(t, layout.Channels, want)
}

func TestLoadContainerStreamsWAVFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "master.wav")
	if err := os.WriteFile(path, makeExtensibleWAV(6, 0x0000003F), 0o644); err != nil {
		t.Fatal(err)
	}
	layout, err := LoadContainer(path)
	if err != nil {
		t.Fatalf("LoadContainer() error = %v", err)
	}
	want := []Label{LabelLeft, LabelRight, LabelCenter, LabelLFE,
		LabelLeftSurround, LabelRightSurround}
	assertLabels(t, layout.Channels, want)
}

func TestParseWAVSevenOneChannelOrder(t *testing.T) {
	layout, err := ParseContainer(makeExtensibleWAV(8, 0x0000063F), "test71.wav")
	if err != nil {
		t.Fatalf("ParseContainer() error = %v", err)
	}
	want := []Label{LabelLeft, LabelRight, LabelCenter, LabelLFE,
		LabelLeftRearSurround, LabelRightRearSurround,
		LabelLeftSurround, LabelRightSurround}
	assertLabels(t, layout.Channels, want)
}

func TestParseWAVRejectsMaskCountMismatch(t *testing.T) {
	_, err := ParseContainer(makeExtensibleWAV(6, 0x0000000F), "bad.wav")
	if err == nil {
		t.Fatal("ParseContainer() accepted a mask with too few channels")
	}
}

func TestParseJSONContainerWithMask(t *testing.T) {
	data := []byte(`{
		"format": "MOV",
		"codec": "pcm_s24le",
		"channel_count": 6,
		"channel_mask": "0x3f"
	}`)
	layout, err := ParseContainer(data, "container.json")
	if err != nil {
		t.Fatalf("ParseContainer() error = %v", err)
	}
	want := []Label{LabelLeft, LabelRight, LabelCenter, LabelLFE,
		LabelLeftSurround, LabelRightSurround}
	assertLabels(t, layout.Channels, want)
}

func TestParseJSONContainerNumericChannelsField(t *testing.T) {
	data := []byte(`{
		"codec": "aac",
		"channels": 2,
		"channel_order": ["L", "R"]
	}`)
	layout, err := ParseContainer(data, "container.json")
	if err != nil {
		t.Fatalf("ParseContainer() error = %v", err)
	}
	assertLabels(t, layout.Channels, []Label{LabelLeft, LabelRight})
}

func TestParseJSONContainerAdaptsLegacyBackLabels(t *testing.T) {
	data := []byte(`{
		"channel_count": 6,
		"channel_order": ["FL", "FR", "FC", "LFE", "BL", "BR"]
	}`)
	layout, err := ParseContainer(data, "container.json")
	if err != nil {
		t.Fatalf("ParseContainer() error = %v", err)
	}
	want := []Label{LabelLeft, LabelRight, LabelCenter, LabelLFE,
		LabelLeftSurround, LabelRightSurround}
	assertLabels(t, layout.Channels, want)
}

func TestParseJSONContainerRejectsMaskConflict(t *testing.T) {
	data := []byte(`{
		"channel_count": 6,
		"channel_mask": "0x3f",
		"channel_order": ["FL", "FR", "C", "LFE", "BL", "BR"]
	}`)
	// BL/BR adapt to Ls/Rs, so this particular descriptor agrees with 0x3F.
	if _, err := ParseContainer(data, "container.json"); err != nil {
		t.Fatalf("compatible mask/order rejected: %v", err)
	}

	bad := []byte(`{
		"channel_count": 6,
		"channel_mask": "0x3f",
		"channel_order": ["FL", "FR", "SL", "LFE", "C", "SR"]
	}`)
	_, err := ParseContainer(bad, "container.json")
	if err == nil {
		t.Fatal("ParseContainer() accepted conflicting mask and order")
	}
}

func assertLabels(t *testing.T, channels []Channel, want []Label) {
	t.Helper()
	if len(channels) != len(want) {
		t.Fatalf("got %d channels, want %d", len(channels), len(want))
	}
	for i, label := range want {
		if channels[i].Label != label {
			t.Errorf("channel %d label = %s, want %s", i+1, channels[i].Label, label)
		}
		if channels[i].Index != i || channels[i].Position != i+1 {
			t.Errorf("channel %d has index/position %d/%d", i+1, channels[i].Index, channels[i].Position)
		}
	}
}

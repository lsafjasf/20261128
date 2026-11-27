package dsd

import (
	"bytes"
	"testing"
)

func TestDFFByteInterleavedPartialFrame(t *testing.T) {
	payload := []byte{
		0x10, 0x11, 0x12,
		0x20, 0x21, 0x22,
		0x30, 0x31, 0x32,
		0x40, 0x41,
	}
	file := makeDFF([]string{"MLFT", "MRGT", "C   "}, 2822400, payload)
	manifest, err := ParseBytes(file)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Format != FormatDFF || manifest.Channels != 3 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if manifest.Packing != PackingByteInterleaved || manifest.BitOrder != BitOrderMSBFirst {
		t.Fatalf("wrong DFF packing: %s %s", manifest.Packing, manifest.BitOrder)
	}
	wantSamples := []int64{32, 32, 24}
	wantBytes := [][]byte{
		{0x10, 0x20, 0x30, 0x40},
		{0x11, 0x21, 0x31, 0x41},
		{0x12, 0x22, 0x32},
	}
	table, err := channelValues(manifest, file)
	if err != nil {
		t.Fatal(err)
	}
	for i := range wantBytes {
		if manifest.ChannelTables[i].ValidSamples != wantSamples[i] {
			t.Errorf("channel %d samples = %d, want %d",
				i, manifest.ChannelTables[i].ValidSamples, wantSamples[i])
		}
		if !bytes.Equal(table[i], wantBytes[i]) {
			t.Errorf("channel %d samples = %x, want %x", i, table[i], wantBytes[i])
		}
	}
	if manifest.ChannelTables[2].MissingBytes != 1 {
		t.Fatalf("channel 2 missing bytes = %d, want 1", manifest.ChannelTables[2].MissingBytes)
	}
	if got := countPadding(manifest, PaddingMissingChannel); got != 1 {
		t.Fatalf("missing-channel padding records = %d, want 1", got)
	}
	if got := countPadding(manifest, PaddingChunkAlignment); got == 0 {
		t.Fatal("odd DFF chunks must expose their alignment padding")
	}
}

func TestDFFMultipleAudioChunksKeepChannelPhase(t *testing.T) {
	file := makeDFF(
		[]string{"L   ", "R   "},
		5644800,
		[]byte{0x01, 0x81, 0x02, 0x82},
		[]byte{0x03, 0x83},
	)
	manifest, err := ParseBytes(file)
	if err != nil {
		t.Fatal(err)
	}
	table, err := channelValues(manifest, file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(table[0], []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("left = %x", table[0])
	}
	if !bytes.Equal(table[1], []byte{0x81, 0x82, 0x83}) {
		t.Fatalf("right = %x", table[1])
	}
	if manifest.ChannelTables[0].Segments[1].FirstSample != 16 {
		t.Fatalf("second chunk starts at sample %d, want 16",
			manifest.ChannelTables[0].Segments[1].FirstSample)
	}
}

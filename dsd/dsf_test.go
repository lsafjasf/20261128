package dsd

import (
	"bytes"
	"testing"
)

func TestDSFPaddedFinalBlocks(t *testing.T) {
	data := []byte{
		0x10, 0x11, 0x12,
		0x20, 0x21, 0x22,
		0x30, 0xfe, 0xfe,
		0x40, 0xfd, 0xfd,
	}
	file := makeDSF(2, 3, 30, data)
	manifest, err := ParseBytes(file)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.BitOrder != BitOrderLSBFirst || manifest.BlockSize != 3 {
		t.Fatalf("unexpected DSF properties: %+v", manifest)
	}
	table, err := channelValues(manifest, file)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]byte{{0x10, 0x11, 0x12, 0x30}, {0x20, 0x21, 0x22, 0x40}}
	for i := range want {
		if !bytes.Equal(table[i], want[i]) {
			t.Errorf("channel %d = %x, want %x", i, table[i], want[i])
		}
		if manifest.ChannelTables[i].ValidSamples != 30 {
			t.Errorf("channel %d samples = %d, want 30", i, manifest.ChannelTables[i].ValidSamples)
		}
		if manifest.ChannelTables[i].PaddingBytes != 2 {
			t.Errorf("channel %d byte padding = %d, want 2",
				i, manifest.ChannelTables[i].PaddingBytes)
		}
		if manifest.ChannelTables[i].PaddingBits != 2 {
			t.Errorf("channel %d bit padding = %d, want 2",
				i, manifest.ChannelTables[i].PaddingBits)
		}
		if manifest.ChannelTables[i].Segments[0].TailLayout != "block-padded" {
			t.Errorf("channel %d tail layout = %q", i,
				manifest.ChannelTables[i].Segments[0].TailLayout)
		}
	}
	if got := countPadding(manifest, PaddingDSFBlock); got != 2 {
		t.Fatalf("block padding records = %d, want 2", got)
	}
	if got := countPadding(manifest, PaddingTailBits); got != 2 {
		t.Fatalf("tail-bit records = %d, want 2", got)
	}
}

func TestDSFTightFinalBlocksWithoutPadding(t *testing.T) {
	data := []byte{
		0x10, 0x11, 0x12,
		0x20, 0x21, 0x22,
		0x30,
		0x40,
	}
	file := makeDSF(2, 3, 30, data)
	manifest, err := ParseBytes(file)
	if err != nil {
		t.Fatal(err)
	}
	table, err := channelValues(manifest, file)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]byte{{0x10, 0x11, 0x12, 0x30}, {0x20, 0x21, 0x22, 0x40}}
	for i := range want {
		if !bytes.Equal(table[i], want[i]) {
			t.Errorf("channel %d = %x, want %x", i, table[i], want[i])
		}
		segment := manifest.ChannelTables[i].Segments[0]
		if segment.TailLayout != "tail-packed" {
			t.Errorf("channel %d tail layout = %q", i, segment.TailLayout)
		}
		if segment.PaddingBytes != 0 || manifest.ChannelTables[i].MissingBytes != 0 {
			t.Errorf("channel %d unexpectedly padded: %+v", i, segment)
		}
	}
	if got := countPadding(manifest, PaddingDSFBlock); got != 0 {
		t.Fatalf("block padding records = %d, want 0", got)
	}
}

func TestDSFMultiChannelVariableBlockSize(t *testing.T) {
	data := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05,
		0x81, 0x82, 0x83, 0x84, 0x85,
		0x41, 0x42, 0x43, 0x44, 0x45,
		0xc1, 0xc2, 0xc3, 0xfa, 0xfb,
		0x31, 0x32, 0x33, 0xfc, 0xfd,
		0xb1, 0xb2, 0xb3, 0xfe, 0xff,
	}
	file := makeDSF(3, 5, 63, data)
	manifest, err := ParseBytes(file)
	if err != nil {
		t.Fatal(err)
	}
	table, err := channelValues(manifest, file)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]byte{
		{0x01, 0x02, 0x03, 0x04, 0x05, 0xc1, 0xc2, 0xc3},
		{0x81, 0x82, 0x83, 0x84, 0x85, 0x31, 0x32, 0x33},
		{0x41, 0x42, 0x43, 0x44, 0x45, 0xb1, 0xb2, 0xb3},
	}
	for i := range want {
		if !bytes.Equal(table[i], want[i]) {
			t.Errorf("channel %d = %x, want %x", i, table[i], want[i])
		}
	}
	if manifest.DeclaredSamples != 63 {
		t.Fatalf("declared samples = %d", manifest.DeclaredSamples)
	}
	if manifest.ChannelTables[2].PaddingBytes != 2 {
		t.Fatalf("channel 2 padding = %d, want 2", manifest.ChannelTables[2].PaddingBytes)
	}
	if manifest.ChannelTables[2].PaddingBits != 1 {
		t.Fatalf("channel 2 bit padding = %d, want 1", manifest.ChannelTables[2].PaddingBits)
	}
}

func TestDSFTruncatedTailMarksMissingPerChannel(t *testing.T) {
	data := []byte{
		0x10, 0x11, 0x12,
		0x20, 0x21, 0x22,
		0x30,
	}
	file := makeDSF(2, 3, 30, data)
	manifest, err := ParseBytes(file)
	if err != nil {
		t.Fatal(err)
	}
	table, err := channelValues(manifest, file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(table[0], []byte{0x10, 0x11, 0x12, 0x30}) {
		t.Fatalf("left = %x", table[0])
	}
	if !bytes.Equal(table[1], []byte{0x20, 0x21, 0x22}) {
		t.Fatalf("right = %x", table[1])
	}
	if manifest.ChannelTables[1].MissingBytes != 1 {
		t.Fatalf("right missing bytes = %d, want 1", manifest.ChannelTables[1].MissingBytes)
	}
	if got := countPadding(manifest, PaddingMissingData); got != 1 {
		t.Fatalf("missing-data records = %d, want 1", got)
	}
}

func TestDSFTrailingDataIsNotSamples(t *testing.T) {
	data := []byte{
		0x10, 0x11, 0x12,
		0x20, 0x21, 0x22,
		0x30, 0xfe, 0xfe,
		0x40, 0xfd, 0xfd,
		0xaa, 0xbb,
	}
	file := makeDSF(2, 3, 30, data)
	manifest, err := ParseBytes(file)
	if err != nil {
		t.Fatal(err)
	}
	table, err := channelValues(manifest, file)
	if err != nil {
		t.Fatal(err)
	}
	for i := range table {
		if len(table[i]) != 4 {
			t.Fatalf("channel %d has %d bytes, want 4", i, len(table[i]))
		}
	}
	if got := countPadding(manifest, PaddingTrailingData); got != 1 {
		t.Fatalf("trailing-data records = %d, want 1", got)
	}
}

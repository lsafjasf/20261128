package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTableAndExtract(t *testing.T) {
	data := []byte{
		0x10, 0x11, 0x12,
		0x20, 0x21, 0x22,
		0x30, 0xfe, 0xfe,
		0x40, 0xfd, 0xfd,
	}
	file := testDSF(2, 3, 30, data)
	path := filepath.Join(t.TempDir(), "tail.dsf")
	if err := os.WriteFile(path, file, 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, reader, closeReader, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeReader()
	var table strings.Builder
	printManifest(&table, manifest)
	if !strings.Contains(table.String(), "packed-byte-tail-bits") {
		t.Fatalf("table does not expose bit padding:\n%s", table.String())
	}
	if !strings.Contains(table.String(), "dsf-block-tail") {
		t.Fatalf("table does not expose block padding:\n%s", table.String())
	}

	dir := t.TempDir()
	if err := extract(manifest, reader, path, dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "tail.ch0.raw"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x10, 0x11, 0x12, 0x30}; !bytes.Equal(got, want) {
		t.Fatalf("extracted %x, want %x", got, want)
	}
}

func testDSF(channels, blockSize int, samples uint64, data []byte) []byte {
	le32 := func(v uint32) []byte {
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, v)
		return buf
	}
	le64 := func(v uint64) []byte {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, v)
		return buf
	}
	fileSize := 92 + len(data)
	fmtBody := bytes.Join([][]byte{
		le32(1), le32(0), le32(2), le32(uint32(channels)),
		le32(2822400), le32(1), le64(samples), le32(uint32(blockSize)), le32(0),
	}, nil)
	return bytes.Join([][]byte{
		[]byte("DSD "), le64(28), le64(uint64(fileSize)), le64(0),
		[]byte("fmt "), le64(52), fmtBody,
		[]byte("data"), le64(uint64(12 + len(data))), data,
	}, nil)
}

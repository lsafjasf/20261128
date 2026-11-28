package wavinfo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func buildRIFF(formType string, chunks ...Chunk) []byte {
	var body bytes.Buffer
	body.WriteString(formType)
	for _, c := range chunks {
		writeChunk(&body, c.ID, c.Data)
	}
	var out bytes.Buffer
	out.WriteString("RIFF")
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(body.Len()))
	out.Write(size[:])
	out.Write(body.Bytes())
	return out.Bytes()
}

func TestParseRIFFRoundTrip(t *testing.T) {
	// Odd-sized data chunk exercises the pad byte path.
	original := buildRIFF("WAVE",
		Chunk{ID: "fmt ", Data: bytes.Repeat([]byte{0xab}, 16)},
		Chunk{ID: "data", Data: []byte{1, 2, 3}},
		Chunk{ID: "JUNK", Data: []byte("x")},
	)
	file, err := ParseRIFF(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("ParseRIFF: %v", err)
	}
	if file.FormType != "WAVE" {
		t.Fatalf("form type = %q", file.FormType)
	}
	if ids := chunkIDs(file.chunks); len(ids) != 3 || ids[0] != "fmt " || ids[2] != "JUNK" {
		t.Fatalf("chunk ids = %v", ids)
	}

	var out bytes.Buffer
	if _, err := file.WriteTo(&out); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !bytes.Equal(out.Bytes(), original) {
		t.Fatalf("untouched RIFF did not round-trip byte-for-byte")
	}
}

func TestParseRIFFPreservesTrailingBytes(t *testing.T) {
	original := buildRIFF("WAVE", Chunk{ID: "data", Data: []byte{1, 2}})
	original = append(original, 0xde, 0xad, 0xbe, 0xef)

	file, err := ParseRIFF(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("ParseRIFF: %v", err)
	}
	if !bytes.Equal(file.trailing, []byte{0xde, 0xad, 0xbe, 0xef}) {
		t.Fatalf("trailing = % x", file.trailing)
	}
	var out bytes.Buffer
	if _, err := file.WriteTo(&out); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !bytes.Equal(out.Bytes(), original) {
		t.Fatalf("trailing bytes were not preserved")
	}
}

func TestParseRIFFErrors(t *testing.T) {
	cases := map[string][]byte{
		"empty":          nil,
		"wrong magic":    []byte("RIFX....WAVE"),
		"truncated head": []byte("RIFF\x10\x00\x00\x00WA"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseRIFF(bytes.NewReader(data)); err == nil {
				t.Fatal("expected error")
			}
		})
	}

	truncated := buildRIFF("WAVE", Chunk{ID: "data", Data: []byte{1, 2}})
	// Claim more bytes than actually present.
	binary.LittleEndian.PutUint32(truncated[4:8], 999)
	_, err := ParseRIFF(bytes.NewReader(truncated))
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}

func TestDirtyChunkReSerialized(t *testing.T) {
	original := buildRIFF("WAVE",
		Chunk{ID: "fmt ", Data: []byte{1, 2, 3}},
		Chunk{ID: "data", Data: []byte{9}},
	)
	file, err := ParseRIFF(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	file.chunks[0].Data = []byte{4, 5, 6, 7}
	file.chunks[0].MarkDirty()

	var out bytes.Buffer
	if _, err := file.WriteTo(&out); err != nil {
		t.Fatal(err)
	}
	reparsed, err := ParseRIFF(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reparsed.chunks[0].Data, []byte{4, 5, 6, 7}) {
		t.Fatalf("dirty data = %v", reparsed.chunks[0].Data)
	}
	// The untouched data chunk keeps its original raw encoding.
	if !bytes.Equal(reparsed.chunks[1].raw, file.chunks[1].raw) {
		t.Fatal("untouched chunk raw bytes changed")
	}
	// And the whole output re-parses to the same declared size.
	declared := binary.LittleEndian.Uint32(out.Bytes()[4:8])
	if int(declared) != len(out.Bytes())-8 {
		t.Fatalf("riff size field %d does not match body %d", declared, len(out.Bytes())-8)
	}
}

func TestParseChunksWithConsecutiveOddPayloads(t *testing.T) {
	// Two back-to-back odd-sized chunks force the parser to apply word
	// alignment on absolute offsets rather than payload lengths.
	original := buildRIFF("WAVE",
		Chunk{ID: "aaaa", Data: []byte{1}},
		Chunk{ID: "bbbb", Data: []byte{2, 3, 4}},
		Chunk{ID: "cccc", Data: []byte{5, 6}},
		Chunk{ID: "dddd", Data: []byte{7}},
	)
	file, err := ParseRIFF(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		id   string
		data []byte
	}{
		{"aaaa", []byte{1}},
		{"bbbb", []byte{2, 3, 4}},
		{"cccc", []byte{5, 6}},
		{"dddd", []byte{7}},
	}
	if len(file.chunks) != len(want) {
		t.Fatalf("parsed %d chunks, want %d", len(file.chunks), len(want))
	}
	for i, w := range want {
		if file.chunks[i].ID != w.id || !bytes.Equal(file.chunks[i].Data, w.data) {
			t.Fatalf("chunk %d = %+v, want %s %v", i, file.chunks[i], w.id, w.data)
		}
	}
	var out bytes.Buffer
	if _, err := file.WriteTo(&out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), original) {
		t.Fatal("consecutive odd chunks did not round-trip byte-for-byte")
	}
}

func chunkIDs(chunks []Chunk) []string {
	ids := make([]string, len(chunks))
	for i, c := range chunks {
		ids[i] = c.ID
	}
	return ids
}

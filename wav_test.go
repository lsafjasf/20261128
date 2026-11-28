package wavinfo

import (
	"bytes"
	"reflect"
	"testing"
)

func buildWAV(t *testing.T, infoEntries []Entry, extraChunks ...Chunk) []byte {
	t.Helper()
	chunks := []Chunk{
		{ID: "fmt ", Data: bytes.Repeat([]byte{7}, 16)},
	}
	if infoEntries != nil {
		var infoData bytes.Buffer
		infoData.WriteString("INFO")
		for _, e := range infoEntries {
			payload := e.payload
			if payload == nil {
				payload = append([]byte(e.Text), 0)
			}
			writeChunk(&infoData, e.ID, payload)
		}
		chunks = append(chunks, Chunk{ID: "LIST", Data: infoData.Bytes()})
	}
	// Odd-sized audio data to prove pad handling stays untouched.
	chunks = append(chunks, Chunk{ID: "data", Data: []byte{0xde, 0xad, 0xbe, 0xef, 0x42}})
	chunks = append(chunks, extraChunks...)
	return buildRIFF("WAVE", chunks...)
}

func TestRewriteKeepsDataChunkIdentical(t *testing.T) {
	original := buildWAV(t, []Entry{
		{ID: "INAM", Text: "old title"},
		{ID: "ZZZZ", payload: []byte("keep me\x00")},
	})

	wav, err := Parse(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	if err := wav.Info().Set("IART", "new artist"); err != nil {
		t.Fatal(err)
	}
	out, err := wav.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	before, err := ParseRIFF(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	after, err := ParseRIFF(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}

	// Every non-INFO chunk must retain its exact on-disk encoding.
	if len(before.chunks) != len(after.chunks) {
		t.Fatalf("chunk count changed: %d -> %d", len(before.chunks), len(after.chunks))
	}
	for i, b := range before.chunks {
		a := after.chunks[i]
		if b.ID == "LIST" {
			if !bytes.Equal(a.Data[0:4], []byte("INFO")) {
				t.Fatal("LIST chunk lost its INFO type")
			}
			continue
		}
		if !bytes.Equal(a.raw, b.raw) {
			t.Fatalf("chunk %s raw bytes changed", b.ID)
		}
	}

	// RIFF size header stays consistent with the body.
	if got := riffSize(out); got != len(out)-8 {
		t.Fatalf("riff size %d, body %d", got, len(out)-8)
	}

	// Re-parsed INFO carries the edit, keeps the unknown field and is ordered.
	reparsed, err := Parse(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	ids := entryIDs(reparsed.Info().Entries())
	want := []string{"IART", "INAM", "ZZZZ"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("entry order = %v want %v", ids, want)
	}
	if got, _ := reparsed.Info().Get("ZZZZ"); got != "keep me" {
		t.Fatalf("unknown field text = %q", got)
	}
}

func TestInsertInfoBeforeData(t *testing.T) {
	original := buildWAV(t, nil)
	wav, err := Parse(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	if err := wav.Info().Set("INAM", "created"); err != nil {
		t.Fatal(err)
	}
	out, err := wav.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := Parse(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	ids := reparsed.ChunkIDs()
	want := []string{"fmt ", "LIST", "data"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("chunk layout = %v want %v", ids, want)
	}
	if got, _ := reparsed.Info().Get("INAM"); got != "created" {
		t.Fatalf("INAM = %q", got)
	}

	// Data chunk still byte-identical after insertion.
	before, _ := ParseRIFF(bytes.NewReader(original))
	for _, c := range reparsed.riff.chunks {
		if c.ID != "data" {
			continue
		}
		if !bytes.Equal(c.raw, before.chunks[1].raw) {
			t.Fatal("data chunk bytes changed when inserting INFO")
		}
	}
}

func TestRejectNonWAVE(t *testing.T) {
	avi := buildRIFF("AVI ", Chunk{ID: "data", Data: []byte{1}})
	if _, err := Parse(bytes.NewReader(avi)); err == nil {
		t.Fatal("expected form type error")
	}
}

func TestResolveField(t *testing.T) {
	cases := map[string]string{
		"inam":  "INAM",
		"INAM":  "INAM",
		"Title": "INAM",
		"ALBUM": "IPRD",
		"genre": "IGNR",
	}
	for in, want := range cases {
		got, ok := ResolveField(in)
		if !ok || got != want {
			t.Fatalf("ResolveField(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if id, ok := ResolveField("weird-custom"); ok || id != "weird-custom" {
		t.Fatalf("unresolved key handling = %q,%v", id, ok)
	}
}

func TestParseFileName(t *testing.T) {
	got := ParseFileName("/music/Album Artist - Song Name.wav")
	if got["INAM"] != "Album Artist" || got["IART"] != "Song Name" {
		t.Fatalf("parsed = %v", got)
	}
	single := ParseFileName("Just Title.wav")
	if single["INAM"] != "Just Title" {
		t.Fatalf("single segment = %v", single)
	}
}

func TestApplyProjectFieldsConflicts(t *testing.T) {
	original := buildWAV(t, []Entry{{ID: "INAM", Text: "embedded title"}})

	project := map[string]string{
		"title":         "project title",
		"genre":         "Rock",
		"mystery-field": "ignored",
	}
	fileName := map[string]string{"INAM": "file title"}

	t.Run("embedded wins by default", func(t *testing.T) {
		wav, _ := Parse(bytes.NewReader(original))
		res, err := wav.ApplyProjectFields(project, fileName, PreferEmbedded)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := wav.Info().Get("INAM"); got != "embedded title" {
			t.Fatalf("INAM = %q", got)
		}
		if got, _ := wav.Info().Get("IGNR"); got != "Rock" {
			t.Fatalf("IGNR = %q", got)
		}
		if len(res.Conflicted) != 1 || res.Conflicted[0].Winner != "embedded" {
			t.Fatalf("conflicts = %+v", res.Conflicted)
		}
		if !reflect.DeepEqual(res.Skipped, []string{"mystery-field"}) {
			t.Fatalf("skipped = %v", res.Skipped)
		}
	})

	t.Run("project preference overrides embedded", func(t *testing.T) {
		wav, _ := Parse(bytes.NewReader(original))
		_, err := wav.ApplyProjectFields(project, fileName, PreferProject)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := wav.Info().Get("INAM"); got != "project title" {
			t.Fatalf("INAM = %q", got)
		}
	})

	t.Run("filename preference wins", func(t *testing.T) {
		wav, _ := Parse(bytes.NewReader(original))
		_, err := wav.ApplyProjectFields(project, fileName, PreferFileName)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := wav.Info().Get("INAM"); got != "file title" {
			t.Fatalf("INAM = %q", got)
		}
	})
}

func TestApplyProjectFieldsFillsMissingFromFileName(t *testing.T) {
	original := buildWAV(t, nil)
	wav, err := Parse(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	// No project value for INAM, but the file name supplies one; embedded
	// preference must still accept the external fill-in.
	res, err := wav.ApplyProjectFields(
		map[string]string{"genre": "Jazz"},
		map[string]string{"INAM": "name-derived title"},
		PreferEmbedded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := wav.Info().Get("INAM"); got != "name-derived title" {
		t.Fatalf("INAM = %q", got)
	}
	if len(res.Conflicted) != 0 {
		t.Fatalf("unexpected conflicts: %+v", res.Conflicted)
	}
}

func entryIDs(entries []Entry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}

func riffSize(b []byte) int {
	return int(b[4]) | int(b[5])<<8 | int(b[6])<<16 | int(b[7])<<24
}

package wavinfo

import (
	"bytes"
	"testing"
)

func buildInfoList(t *testing.T, entries ...Entry) *InfoList {
	t.Helper()
	var data bytes.Buffer
	data.WriteString("INFO")
	for _, e := range entries {
		payload := e.payload
		if payload == nil {
			payload = append([]byte(e.Text), 0)
		}
		writeChunk(&data, e.ID, payload)
	}
	list, err := parseInfoList(data.Bytes())
	if err != nil {
		t.Fatalf("parseInfoList: %v", err)
	}
	return list
}

func TestParseInfoStripsSingleNUL(t *testing.T) {
	list := buildInfoList(t,
		Entry{ID: "IART", payload: []byte("Bandsmith\x00")},
		Entry{ID: "INAM", payload: []byte("no terminator")},
		Entry{ID: "ICMT", payload: []byte("double\x00\x00")},
	)
	cases := map[string]string{
		"IART": "Bandsmith",
		"INAM": "no terminator",
		"ICMT": "double\x00",
	}
	for id, want := range cases {
		got, ok := list.Get(id)
		if !ok || got != want {
			t.Fatalf("Get(%s) = %q,%v want %q", id, got, ok, want)
		}
	}
}

func TestUnknownFieldsPreserved(t *testing.T) {
	list := buildInfoList(t,
		Entry{ID: "IART", Text: "Artist"},
		Entry{ID: "XYZ9", payload: []byte("bespoke\x00")},
		Entry{ID: "ABCD", payload: []byte{0x01, 0x02, 0x03}},
	)
	if list.Entries()[1].Known() {
		t.Fatal("XYZ9 must be treated as unknown")
	}

	encoded := list.encode()
	reparsed, err := parseInfoList(encoded)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Entry{}
	for _, e := range reparsed.Entries() {
		byID[e.ID] = e
	}
	if !bytes.Equal(byID["XYZ9"].payload, []byte("bespoke\x00")) {
		t.Fatalf("unknown text payload lost: % x", byID["XYZ9"].payload)
	}
	if !bytes.Equal(byID["ABCD"].payload, []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("unknown binary payload lost: % x", byID["ABCD"].payload)
	}
}

func TestEncodeCanonicalOrder(t *testing.T) {
	list := buildInfoList(t,
		// Deliberately shuffled, plus unknown entries interspersed.
		Entry{ID: "ITCH", Text: "tech"},
		Entry{ID: "ZZZZ", Text: "late"},
		Entry{ID: "IART", Text: "art"},
		Entry{ID: "AAAA", Text: "unknown-first"},
		Entry{ID: "INAM", Text: "title"},
	)
	encoded := list.encode()
	reparsed, err := parseInfoList(encoded)
	if err != nil {
		t.Fatal(err)
	}

	got := make([]string, 0, len(reparsed.Entries()))
	for _, e := range reparsed.Entries() {
		got = append(got, e.ID)
	}
	// Unknown fields trail the known ones but keep parse-order relative
	// positions: ZZZZ appeared before AAAA in the shuffled input.
	want := []string{"IART", "INAM", "ITCH", "ZZZZ", "AAAA"}
	if len(got) != len(want) {
		t.Fatalf("ids = %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v want %v", got, want)
		}
	}
}

func TestSetDeleteAndDuplicates(t *testing.T) {
	list := buildInfoList(t,
		Entry{ID: "IART", Text: "first"},
		Entry{ID: "IART", Text: "second"},
	)
	if got, _ := list.Get("IART"); got != "first" {
		t.Fatalf("Get before set = %q", got)
	}
	if err := list.Set("IART", "only"); err != nil {
		t.Fatal(err)
	}
	if len(list.Entries()) != 1 || list.Entries()[0].Text != "only" {
		t.Fatalf("Set did not collapse duplicates: %+v", list.Entries())
	}
	if got := list.Entries()[0].payload; !bytes.Equal(got, []byte("only\x00")) {
		t.Fatalf("Set payload = % x", got)
	}
	if err := list.Set("BAD", "x"); err == nil {
		t.Fatal("expected error for 3-byte id")
	}
	if !list.Delete("IART") || len(list.Entries()) != 0 {
		t.Fatal("Delete failed")
	}
	if list.Delete("IART") {
		t.Fatal("Delete on missing field must report false")
	}
}

func TestSetRawRoundTrip(t *testing.T) {
	list := &InfoList{}
	if err := list.SetRaw("XYZZ", []byte{0x10, 0x20}); err != nil {
		t.Fatal(err)
	}
	reparsed, err := parseInfoList(list.encode())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reparsed.Entries()[0].payload, []byte{0x10, 0x20}) {
		t.Fatalf("raw payload = % x", reparsed.Entries()[0].payload)
	}
}

func TestTruncatedInfoListRejected(t *testing.T) {
	bad := []byte("INFO")
	bad = append(bad, 'I', 'A', 'R', 'T', 0x10, 0, 0, 0) // claims 16 bytes, has none
	if _, err := parseInfoList(bad); err == nil {
		t.Fatal("expected truncation error")
	}
}

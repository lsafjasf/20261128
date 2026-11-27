package webvtt

import (
	"strings"
	"testing"
	"time"
)

func TestParseDocument(t *testing.T) {
	const source = "\ufeffWEBVTT - demo\r\nKind: captions\r\n\r\n" + `intro
00:00:01.250 --> 00:00:03.500 align:start position:10%
<v Bob>Hi &amp; bye
second

00:04.000 --> 00:05.000
<lang fr>Salut</lang>
`
	doc, err := ParseString(source)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Header != "WEBVTT - demo" {
		t.Fatalf("header = %q", doc.Header)
	}
	if len(doc.Cues) != 2 {
		t.Fatalf("got %d cues, want 2", len(doc.Cues))
	}
	first := doc.Cues[0]
	if first.ID != "intro" {
		t.Fatalf("id = %q", first.ID)
	}
	if first.Start != 1250*time.Millisecond || first.End != 3500*time.Millisecond {
		t.Fatalf("timing = %v -> %v", first.Start, first.End)
	}
	if first.Settings != "align:start position:10%" {
		t.Fatalf("settings = %q", first.Settings)
	}
	if first.Payload != "<v Bob>Hi &amp; bye\nsecond" {
		t.Fatalf("payload = %q", first.Payload)
	}
	if got := first.Text(""); got != "Hi & bye\nsecond" {
		t.Fatalf("text = %q", got)
	}
	if got := doc.Cues[1].Text("fr"); got != "Salut" {
		t.Fatalf("second cue text = %q", got)
	}
}

func TestParseDocumentSkipsNoteAndStyleBlocks(t *testing.T) {
	const source = `WEBVTT

NOTE this is ignored
still ignored

STYLE
::cue { color: lime }

00:00.000 --> 00:01.000
visible
`
	doc, err := ParseString(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cues) != 1 || doc.Cues[0].Payload != "visible" {
		t.Fatalf("cues = %#v", doc.Cues)
	}
}

func TestParseDocumentRejectsInvalidHeaderAndTiming(t *testing.T) {
	tests := []string{
		"not webvtt\n",
		"WEBVTT\n\n00:00.000 --> nope\nx\n",
		"WEBVTT\n\n00:02.000 --> 00:01.000\nx\n",
	}
	for _, source := range tests {
		if _, err := ParseString(source); err == nil {
			t.Errorf("ParseString(%q) succeeded, want error", source)
		}
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{"00:01.250", 1250 * time.Millisecond},
		{"01:02:03.004", time.Hour + 2*time.Minute + 3*time.Second + 4*time.Millisecond},
		{"100:00:00.000", 100 * time.Hour},
	}
	for _, test := range tests {
		got, err := ParseTimestamp(test.value)
		if err != nil {
			t.Errorf("ParseTimestamp(%q): %v", test.value, err)
			continue
		}
		if got != test.want {
			t.Errorf("ParseTimestamp(%q) = %v, want %v", test.value, got, test.want)
		}
	}
	for _, value := range []string{"1:02.000", "00:60.000", "00:00:60.000", "00:00.00"} {
		if _, err := ParseTimestamp(value); err == nil {
			t.Errorf("ParseTimestamp(%q) succeeded", value)
		}
	}
}

func TestParseDocumentKeepsRawPayload(t *testing.T) {
	const payload = `<v Alice>one <unknown data-v="1">two</unknown></v>`
	doc, err := ParseString("WEBVTT\n\n00:00.000 --> 00:01.000\n" + payload + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Cues[0].RawText(); got != payload {
		t.Fatalf("raw text = %q", got)
	}
	if !strings.Contains(doc.Cues[0].PlainText(), `<unknown data-v="1">`) {
		t.Fatalf("unknown annotation was lost: %q", doc.Cues[0].PlainText())
	}
}

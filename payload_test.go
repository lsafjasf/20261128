package webvtt

import (
	"strings"
	"testing"
)

func TestParseCuePayloadVoiceAndLanguage(t *testing.T) {
	const payload = `<v Alice><lang en-US>Hello &amp; welcome</lang></v>`
	nodes, err := ParseCuePayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("got %d root nodes, want 1", len(nodes))
	}
	voice := nodes[0]
	if voice.Type != NodeVoice || voice.Voice != "Alice" {
		t.Fatalf("unexpected voice node: %+v", voice)
	}
	if voice.Raw != payload || voice.StartTag != `<v Alice>` || voice.EndTag != `</v>` {
		t.Fatalf("voice raw data was not preserved: %+v", voice)
	}
	if len(voice.Children) != 1 {
		t.Fatalf("got %d voice children, want 1", len(voice.Children))
	}
	language := voice.Children[0]
	if language.Type != NodeLanguage || language.Language != "en-US" {
		t.Fatalf("unexpected language node: %+v", language)
	}
	if got := language.Children[0].Text; got != "Hello & welcome" {
		t.Fatalf("decoded text = %q", got)
	}
	if got := sourceOf(nodes); got != payload {
		t.Fatalf("reconstructed source = %q, want %q", got, payload)
	}
}

func TestParseCuePayloadPreservesUnknownAnnotations(t *testing.T) {
	const payload = `before <kbd data-key="x">key</kbd> <v>bad</v> after`
	nodes, err := ParseCuePayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceOf(nodes); got != payload {
		t.Fatalf("reconstructed source = %q, want %q", got, payload)
	}
	if nodes[1].Type != NodeUnknown || nodes[1].Annotation != `data-key="x"` {
		t.Fatalf("unknown start tag not retained: %+v", nodes[1])
	}
	if got := RenderText(nodes, RenderOptions{}); got != payload {
		t.Fatalf("render with unknown markup = %q, want %q", got, payload)
	}
	if got := RenderText(nodes, RenderOptions{OmitUnknown: true}); got != "before key bad after" {
		t.Fatalf("render without unknown markup = %q", got)
	}
}

func TestParseCuePayloadMalformedAndUnclosedMarkup(t *testing.T) {
	const payload = `<b>one<i>two</b>three</i>`
	nodes, err := ParseCuePayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceOf(nodes); got != payload {
		t.Fatalf("reconstructed source = %q, want %q", got, payload)
	}
	if got := RenderText(nodes, RenderOptions{}); got != "onetwothree</i>" {
		t.Fatalf("render = %q", got)
	}
}

func TestParseCuePayloadLeavesStrayLessThanAsText(t *testing.T) {
	const payload = `a < b &c; &lt;tag&gt;`
	nodes, err := ParseCuePayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := RenderText(nodes, RenderOptions{}); got != `a < b &c; <tag>` {
		t.Fatalf("render = %q", got)
	}
	if got := sourceOf(nodes); got != payload {
		t.Fatalf("reconstructed source = %q, want %q", got, payload)
	}
}

func sourceOf(nodes []Node) string {
	var out strings.Builder
	for _, node := range nodes {
		out.WriteString(node.Raw)
	}
	return out.String()
}

package imsc

import (
	"bytes"
	"strings"
	"testing"
)

const fallbackDoc = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttp="http://www.w3.org/ns/ttml#parameter"
    xmlns:tts="http://www.w3.org/ns/ttml#styling"
    xmlns:smpte="http://www.smpte-ra.org/schemas/2052-1/2010/smpte-tt"
    xmlns:conv="urn:imscconv:v1">
  <head>
    <layout>
      <region xml:id="safe" tts:origin="0px 0px" tts:extent="100px 100px"/>
      <region xml:id="preferred" tts:origin="5px 5px" tts:extent="100px 100px" conv:regionFallback="safe"/>
    </layout>
    <smpte:image xml:id="fallbackImg" smpte:imageType="image/png" smpte:encoding="Base64">UE5HLUZBTExCQUNL</smpte:image>
  </head>
  <body>
    <div xml:id="d">
      <p xml:id="c1" begin="0s" end="1s" region="preferred" conv:imageFallback="#fallbackImg"
         smpte:backgroundImage="images/4k-logo.png"/>
    </div>
  </body>
</tt>`

func TestFallbackChainsSurviveRoundTrip(t *testing.T) {
	first, err := Parse(strings.NewReader(fallbackDoc))
	if err != nil {
		t.Fatal(err)
	}

	if got, ok := first.Resources.Fallback("images/4k-logo.png"); !ok || got != "#fallbackImg" {
		t.Fatalf("resource fallback not built: %q ok=%v", got, ok)
	}
	rg, ok := first.Regions.Get("preferred")
	if !ok || rg.FallbackID != "safe" {
		t.Fatalf("region fallback not built: %+v ok=%v", rg, ok)
	}

	buf := &bytes.Buffer{}
	if err := first.Export(buf, DefaultExportOptions()); err != nil {
		t.Fatal(err)
	}
	second, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}

	if got, ok := second.Resources.Fallback("images/4k-logo.png"); !ok || got != "#fallbackImg" {
		t.Fatalf("resource fallback not restored: %q ok=%v", got, ok)
	}
	chain := second.Resources.Chain("images/4k-logo.png")
	if len(chain) != 2 || chain[1] != "#fallbackImg" {
		t.Fatalf("resource fallback chain not restored: %v", chain)
	}
	rg2, ok := second.Regions.Get("preferred")
	if !ok {
		t.Fatal("preferred region missing after re-import")
	}
	if rg2.FallbackID != "safe" {
		t.Fatalf("region fallback not restored: %+v", rg2)
	}
}

func TestUnknownRegionTerminatesAtDefault(t *testing.T) {
	doc, err := Parse(strings.NewReader(fallbackDoc))
	if err != nil {
		t.Fatal(err)
	}
	rg, err := doc.Regions.Resolve("not-declared")
	if err != nil {
		t.Fatalf("missing region should silently fall back, got error: %v", err)
	}
	if rg.ID != DefaultRegionID {
		t.Fatalf("expected default region terminator, got %q", rg.ID)
	}
}

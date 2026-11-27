package imsc

import (
	"bytes"
	"strings"
	"testing"
)

const sampleDoc = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttp="http://www.w3.org/ns/ttml#parameter"
    xmlns:tts="http://www.w3.org/ns/ttml#styling"
    xmlns:smpte="http://www.smpte-ra.org/schemas/2052-1/2010/smpte-tt"
    xmlns:conv="urn:imscconv:v1"
    ttp:extent="1920px 1080px">
  <head>
    <layout>
      <region xml:id="topLeft" tts:origin="10px 20px" tts:extent="300px 200px"/>
      <region xml:id="topLeftCopy" tts:origin="10px 20px" tts:extent="300px 200px"/>
    </layout>
    <smpte:image xml:id="logoA" smpte:imageType="image/png" smpte:encoding="Base64">UE5HLVdBVEVSTUFSSw==</smpte:image>
  </head>
  <body>
    <div xml:id="d1">
      <p xml:id="cue1" begin="00:00:01:00" end="00:00:02:00" region="topLeft" smpte:backgroundImage="#logoA"/>
      <p xml:id="cue2" begin="00:00:02:00" end="00:00:03:00" region="topLeftCopy" smpte:backgroundImage="#logoA" conv:imageFallback="#logoA"/>
    </div>
  </body>
</tt>`

func TestParseBuildsSharedReferences(t *testing.T) {
	doc, err := Parse(strings.NewReader(sampleDoc))
	if err != nil {
		t.Fatal(err)
	}
	if doc.CueCount() != 2 {
		t.Fatalf("expected 2 cues, got %d", doc.CueCount())
	}
	if doc.Resources.Len() != 1 {
		t.Fatalf("two cues sharing one image must produce 1 resource, got %d", doc.Resources.Len())
	}
	if doc.Regions.Len() != 1 {
		t.Fatalf("identical regions must merge into 1, got %d", doc.Regions.Len())
	}
	if doc.Cues[0].ResourceKey != doc.Cues[1].ResourceKey {
		t.Fatal("cues must point at the same resource key")
	}
	rg, ok := doc.Regions.Get("topLeftCopy")
	if !ok || rg.ID != "topLeft" {
		t.Fatal("merged region alias must resolve to canonical region")
	}
}

func TestRoundTripRestoresReferences(t *testing.T) {
	first, err := Parse(strings.NewReader(sampleDoc))
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	if err := first.Export(buf, DefaultExportOptions()); err != nil {
		t.Fatal(err)
	}

	second, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}

	if second.CueCount() != first.CueCount() {
		t.Fatalf("cue count drift: %d -> %d", first.CueCount(), second.CueCount())
	}
	if second.Resources.Len() != 1 {
		t.Fatalf("re-import must restore single shared resource, got %d", second.Resources.Len())
	}
	if second.Regions.Len() != 1 {
		t.Fatalf("re-import must restore merged region, got %d", second.Regions.Len())
	}
	for i, cue := range second.Cues {
		if cue.ResourceKey != first.Cues[i].ResourceKey {
			t.Fatalf("cue %d resource key drift", i)
		}
	}
	if second.Cues[1].RegionID != "topLeftCopy" {
		t.Fatalf("region alias reference lost, got %q", second.Cues[1].RegionID)
	}
	rg, _ := second.Regions.Get("topLeftCopy")
	if rg == nil || rg.ID != "topLeft" {
		t.Fatal("alias must resolve to the canonical region after re-import")
	}
	if second.Cues[1].FallbackURI != "#logoA" {
		t.Fatalf("fallback link lost: %q", second.Cues[1].FallbackURI)
	}

	// The exported document must contain exactly one image element.
	if count := strings.Count(buf.String(), "<smpte:image"); count != 1 {
		t.Fatalf("expected one shared smpte:image element, got %d", count)
	}
}

func TestRoundTripDataURIMode(t *testing.T) {
	first, err := Parse(strings.NewReader(sampleDoc))
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	if err := first.Export(buf, ExportOptions{DataURI: true}); err != nil {
		t.Fatal(err)
	}
	second, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if second.Resources.Len() != 1 {
		t.Fatalf("identical data URIs must re-merge to one resource, got %d", second.Resources.Len())
	}
	if second.Cues[0].ResourceKey != second.Cues[1].ResourceKey {
		t.Fatal("data URI cues must share a resource key after re-import")
	}
}

func TestExternalURISurvivesRoundTrip(t *testing.T) {
	const doc = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttp="http://www.w3.org/ns/ttml#parameter"
    xmlns:tts="http://www.w3.org/ns/ttml#styling"
    xmlns:smpte="http://www.smpte-ra.org/schemas/2052-1/2010/smpte-tt">
  <head><layout/></head>
  <body><div xml:id="d">
    <p xml:id="c1" begin="0s" end="1s" smpte:backgroundImage="images/logo.png"/>
  </div></body>
</tt>`
	parsed, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Resources.Len() != 1 {
		t.Fatalf("expected one external resource, got %d", parsed.Resources.Len())
	}
	buf := &bytes.Buffer{}
	if err := parsed.Export(buf, DefaultExportOptions()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "images/logo.png") {
		t.Fatal("external URI must be preserved on export")
	}
	second, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if second.Resources.Len() != 1 || second.Cues[0].ImageURI != "images/logo.png" {
		t.Fatal("external reference must be restored after re-import")
	}
}

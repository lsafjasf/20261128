package imsc

import (
	"bytes"
	"strings"
	"testing"
)

const extraAttrsDoc = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttp="http://www.w3.org/ns/ttml#parameter"
    xmlns:tts="http://www.w3.org/ns/ttml#styling"
    xmlns:custom="http://example.com/custom"
    xmlns:smpte="http://www.smpte-ra.org/schemas/2052-1/2010/smpte-tt">
  <head>
    <layout>
      <region xml:id="r1" tts:origin="1px 2px" tts:extent="3px 4px" custom:zIndex="7" custom:name="lower-third"/>
    </layout>
    <smpte:image xml:id="i1" smpte:imageType="image/png" smpte:encoding="Base64">REFMQQ==</smpte:image>
  </head>
  <body><div xml:id="d">
    <p xml:id="c1" begin="0s" end="1s" region="r1" smpte:backgroundImage="#i1"/>
  </div></body>
</tt>`

func TestRegionExtraAttrsRoundTrip(t *testing.T) {
	first, err := Parse(strings.NewReader(extraAttrsDoc))
	if err != nil {
		t.Fatal(err)
	}
	rg, ok := first.Regions.Get("r1")
	if !ok {
		t.Fatal("region r1 missing")
	}
	if got := rg.OtherAttrs["{http://example.com/custom}zIndex"]; got != "7" {
		t.Fatalf("custom attribute not captured: %q", got)
	}

	buf := &bytes.Buffer{}
	if err := first.Export(buf, DefaultExportOptions()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "xmlns:ns1=\"http://example.com/custom\"") ||
		!strings.Contains(out, "ns1:zIndex=\"7\"") ||
		!strings.Contains(out, "ns1:name=\"lower-third\"") {
		t.Fatalf("custom attributes not serialized:\n%s", out)
	}

	second, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rg2, _ := second.Regions.Get("r1")
	if rg2 == nil {
		t.Fatal("region lost after re-import")
	}
	if got := rg2.OtherAttrs["{http://example.com/custom}zIndex"]; got != "7" {
		t.Fatalf("custom attribute not restored: %q", got)
	}
}

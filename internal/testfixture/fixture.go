// Package testfixture generates TTML test material programmatically. Keeping
// fixtures in Go avoids hand-edited XML drifting away from the test cases.
package testfixture

//go:generate go run ../../cmd/ttml-fixture -out ../../testdata/generated

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

// Ruby renders a TTML ruby element. Each layer is one rt line, nearest to the
// base first.
func Ruby(base string, layers ...string) string {
	var builder strings.Builder
	builder.WriteString("<ruby><rb>")
	builder.WriteString(html.EscapeString(base))
	builder.WriteString("</rb>")
	for _, layer := range layers {
		builder.WriteString("<rt>")
		builder.WriteString(html.EscapeString(layer))
		builder.WriteString("</rt>")
	}
	builder.WriteString("</ruby>")
	return builder.String()
}

// NestedRuby renders a two-layer annotation using the nested form permitted by
// TTML processors that treat ruby as inline content.
func NestedRuby(base, innerLayer, outerLayer string) string {
	inner := "<ruby><rb>" + html.EscapeString(base) + "</rb><rt>" +
		html.EscapeString(innerLayer) + "</rt></ruby>"
	return "<ruby><rb>" + inner + "</rb><rt>" +
		html.EscapeString(outerLayer) + "</rt></ruby>"
}

// BR is an explicit TTML line break.
const BR = "<br/>"

// Document wraps generated body content in a complete TTML document.
func Document(body string) []byte {
	return document(body, "40px 260px", "560px 80px")
}

// NarrowDocument forces wrapping by using a region only three CJK ems wide.
func NarrowDocument(body string) []byte {
	return document(body, "10px 80px", "100px 220px")
}

func document(body, origin, extent string) []byte {
	ttml := `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:tts="http://www.w3.org/ns/ttml#styling"
    tts:extent="640px 360px">
  <head>
    <styling>
      <style xml:id="subtitle" tts:fontSize="32px" tts:lineHeight="40px"
             tts:textAlign="center" tts:displayAlign="after"
             tts:rubyPosition="before" tts:rubyAlign="center"/>
    </styling>
    <layout>
      <region xml:id="bottom" style="subtitle"
              tts:origin="` + origin + `" tts:extent="` + extent + `"/>
    </layout>
  </head>
  <body>
    <div>
      <p region="bottom">` + body + `</p>
    </div>
  </body>
</tt>`
	return []byte(ttml)
}

// DoubleLayer returns a document with nested and direct two-layer ruby.
func DoubleLayer() []byte {
	body := NestedRuby("漢", "かん", "han") +
		"字" + Ruby("東", "とう", "tou") + "京" + BR +
		"普通文字" + Ruby("雨", "あめ", "yu")
	return Document(body)
}

// Wrapping returns a document whose ruby base is wider than the region.
func Wrapping() []byte {
	body := Ruby("漢字漢字", "かんじかんじ") +
		"と" + Ruby("日本語", "にほんご") + "の字幕"
	return NarrowDocument(body)
}

// MissingGlyph includes a base rune and an annotation rune that the test face
// intentionally does not support.
func MissingGlyph() []byte {
	return Document("可用" + Ruby("缺", "けつ") + "\U0001F9FF" +
		Ruby("字", "\U0001F9FF"))
}

// SpanRuby exercises the TTML2 tts:ruby container/base/text spelling.
func SpanRuby() []byte {
	body := `<span tts:ruby="container">` +
		`<span tts:ruby="base">漢</span>` +
		`<span tts:ruby="text">かん</span>` +
		`<span tts:ruby="text">han</span>` +
		`</span>` +
		`<span tts:ruby="base">字</span>` +
		`<span tts:ruby="text">じ</span>` +
		`<span tts:ruby="text">ji</span>`
	return Document(body)
}

// PercentageLayout verifies that x and y percentages use their own viewport
// dimensions.
func PercentageLayout() []byte {
	return document("百分比", "10% 20%", "80% 30%")
}

// All returns the named fixture set used by tests and the fixture command.
func All() map[string][]byte {
	return map[string][]byte{
		"double-layer.ttml":  DoubleLayer(),
		"wrapping.ttml":      Wrapping(),
		"missing-glyph.ttml": MissingGlyph(),
		"span-ruby.ttml":     SpanRuby(),
		"percentage.ttml":    PercentageLayout(),
	}
}

// WriteAll writes all generated fixtures into dir.
func WriteAll(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, data := range All() {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

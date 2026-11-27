package layout

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"ttmlruby/internal/testfixture"
)

type testFace struct {
	missing map[rune]bool
}

func (f testFace) Advance(r rune) (float64, bool) {
	if f.missing[r] {
		return 0, false
	}
	switch {
	case r == '\uFFFD':
		return 1, true
	case r == ' ':
		return 0.25, true
	case isWideRune(r):
		return 1, true
	default:
		return 0.55, true
	}
}

func (testFace) Ascent() float64  { return 0.8 }
func (testFace) Descent() float64 { return 0.2 }

func relayout(t *testing.T, data []byte, face MetricFace) *Result {
	t.Helper()
	result, err := LayoutTTML(data, Options{Face: face})
	if err != nil {
		t.Fatalf("LayoutTTML: %v", err)
	}
	return result
}

func near(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestDoubleLayerRubyDoesNotAffectBaselines(t *testing.T) {
	result := relayout(t, testfixture.DoubleLayer(), testFace{})
	wantRegion := Rect{X: 40, Y: 260, Width: 560, Height: 80}
	if result.Region != wantRegion {
		t.Fatalf("region = %+v, want %+v", result.Region, wantRegion)
	}
	if len(result.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(result.Lines))
	}
	if result.Lines[0].Text != "漢字東京" || result.Lines[1].Text != "普通文字雨" {
		t.Fatalf("unexpected logical text: %q / %q", result.Lines[0].Text, result.Lines[1].Text)
	}

	for i, line := range result.Lines {
		wantBaseline := 289.6 + float64(i)*40
		if !near(line.Baseline, wantBaseline) {
			t.Errorf("line %d baseline = %.3f, want %.3f", i, line.Baseline, wantBaseline)
		}
		if !near(line.Box.Height, 40) {
			t.Errorf("line %d box height = %.3f, want 40", i, line.Box.Height)
		}
	}

	layers := map[int]bool{}
	for _, ruby := range result.Lines[0].Ruby {
		layers[ruby.Layer] = true
		if ruby.Layer == 1 {
			for _, lower := range result.Lines[0].Ruby {
				if lower.Layer == 0 && near(lower.Box.X, ruby.Box.X) {
					if ruby.Box.Bottom() > lower.Box.Y+epsilon {
						t.Errorf("outer ruby overlaps inner ruby: %.3f > %.3f", ruby.Box.Bottom(), lower.Box.Y)
					}
				}
			}
		}
	}
	if !layers[0] || !layers[1] {
		t.Fatalf("expected both ruby layers, got %+v", layers)
	}

	for _, line := range result.Lines {
		for _, ruby := range line.Ruby {
			if ruby.Box.Bottom() > line.Box.Y+epsilon {
				t.Errorf("ruby extends into base line box: ruby bottom %.3f, line top %.3f", ruby.Box.Bottom(), line.Box.Y)
			}
			if !containsRect(result.ContentBounds, ruby.Box) {
				t.Errorf("content bounds %+v do not contain ruby box %+v", result.ContentBounds, ruby.Box)
			}
		}
		if !containsRect(result.ContentBounds, line.Box) {
			t.Errorf("content bounds %+v do not contain line box %+v", result.ContentBounds, line.Box)
		}
	}
}

func TestWrappingSplitsRubyLosslessly(t *testing.T) {
	result := relayout(t, testfixture.Wrapping(), testFace{})
	if len(result.Lines) < 3 {
		t.Fatalf("expected wrapping across several lines, got %d: %+v", len(result.Lines), result.Lines)
	}
	var base, ruby strings.Builder
	for i, line := range result.Lines {
		if line.Box.Width > 100+epsilon {
			t.Errorf("line %d width %.2f exceeds region width 100", i, line.Box.Width)
		}
		base.WriteString(line.Text)
		for _, layer := range line.Ruby {
			if layer.Layer != 0 {
				t.Errorf("unexpected ruby layer %d", layer.Layer)
			}
			ruby.WriteString(layer.Text)
		}
	}
	if got, want := base.String(), "漢字漢字と日本語の字幕"; got != want {
		t.Errorf("logical text after wrapping = %q, want %q", got, want)
	}
	if got, want := ruby.String(), "かんじかんじにほんご"; got != want {
		t.Errorf("ruby text after wrapping = %q, want %q", got, want)
	}
	for i := 1; i < len(result.Lines); i++ {
		delta := result.Lines[i].Baseline - result.Lines[i-1].Baseline
		if !near(delta, 40) {
			t.Errorf("baseline delta %d = %.3f, want 40", i, delta)
		}
	}
}

func TestMissingGlyphsAreReportedPerLayer(t *testing.T) {
	face := testFace{missing: map[rune]bool{'\U0001F9FF': true}}
	result := relayout(t, testfixture.MissingGlyph(), face)
	var baseMissing, rubyMissing bool
	for _, missing := range result.Missing {
		switch {
		case missing.Code == "U+1F9FF" && missing.Layer == "base":
			baseMissing = true
		case missing.Code == "U+1F9FF" && missing.Layer == "ruby[0]":
			rubyMissing = true
		}
	}
	if !baseMissing || !rubyMissing {
		t.Fatalf("missing diagnostics = %+v, want base and ruby entries", result.Missing)
	}
	if !strings.Contains(result.Lines[0].Text, "\U0001F9FF") {
		t.Errorf("logical text should preserve missing rune, got %q", result.Lines[0].Text)
	}
}

func TestTTML2SpanRubyForms(t *testing.T) {
	result := relayout(t, testfixture.SpanRuby(), testFace{})
	if len(result.Lines) != 1 || result.Lines[0].Text != "漢字" {
		t.Fatalf("unexpected span-ruby text: %+v", result.Lines)
	}
	layers := map[string]bool{}
	for _, ruby := range result.Lines[0].Ruby {
		layers[ruby.Text] = true
	}
	for _, want := range []string{"かん", "han", "じ", "ji"} {
		if !layers[want] {
			t.Errorf("missing span ruby %q in %+v", want, result.Lines[0].Ruby)
		}
	}
}

func TestPercentageOriginAndExtentUseViewportAxes(t *testing.T) {
	result := relayout(t, testfixture.PercentageLayout(), testFace{})
	want := Rect{X: 64, Y: 72, Width: 512, Height: 108}
	if result.Region != want {
		t.Fatalf("region = %+v, want %+v", result.Region, want)
	}
}

func TestResultJSONContainsRegionAndLineBaselines(t *testing.T) {
	result := relayout(t, testfixture.DoubleLayer(), testFace{})
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, key := range []string{`"region"`, `"contentBounds"`, `"baseline"`, `"lines"`} {
		if !strings.Contains(text, key) {
			t.Errorf("JSON lacks %s: %s", key, text)
		}
	}
}

func TestInvalidSelectionReturnsError(t *testing.T) {
	if _, err := LayoutTTML(testfixture.DoubleLayer(), Options{Paragraph: 4}); err == nil {
		t.Fatal("expected paragraph range error")
	}
	if _, err := LayoutTTML(testfixture.DoubleLayer(), Options{RegionID: "missing"}); err == nil {
		t.Fatal("expected region lookup error")
	}
}

func containsRect(outer, inner Rect) bool {
	return outer.X <= inner.X+epsilon &&
		outer.Y <= inner.Y+epsilon &&
		outer.Right() >= inner.Right()-epsilon &&
		outer.Bottom() >= inner.Bottom()-epsilon
}

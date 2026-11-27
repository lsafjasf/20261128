package webvtt

import "testing"

func TestRenderLanguageFallback(t *testing.T) {
	nodes := ParseCueMarkup(`<lang en>Hello</lang><lang fr>Bonjour</lang>`)
	tests := []struct {
		language string
		want     string
		fallback bool
	}{
		{language: "en", want: "Hello"},
		{language: "EN-us", want: "Hello"},
		{language: "fr-CA", want: "Bonjour"},
		{language: "de", want: "HelloBonjour", fallback: true},
	}
	for _, test := range tests {
		result := Render(nodes, RenderOptions{Language: test.language})
		if result.Text != test.want {
			t.Errorf("Render(%q) = %q, want %q", test.language, result.Text, test.want)
		}
		if result.Fallback != test.fallback {
			t.Errorf("Render(%q).Fallback = %v, want %v", test.language, result.Fallback, test.fallback)
		}
	}
}

func TestRenderLanguageSpecificityAndInheritance(t *testing.T) {
	nodes := ParseCueMarkup(`<lang en>hello <lang en-US>howdy</lang></lang> world`)
	tests := []struct {
		language string
		want     string
	}{
		{language: "en-US", want: "howdy world"},
		{language: "en-GB", want: "hello  world"},
		{language: "fr", want: "hello howdy world"},
	}
	for _, test := range tests {
		if got := RenderText(nodes, RenderOptions{Language: test.language}); got != test.want {
			t.Errorf("Render(%q) = %q, want %q", test.language, got, test.want)
		}
	}
}

func TestRenderKnownMarkupIsNotBodyText(t *testing.T) {
	nodes := ParseCueMarkup(`<v.loud Alice><lang en><b>Hi</b> <i>there</i></lang></v>`)
	result := Render(nodes, RenderOptions{Language: "en"})
	if result.Text != "Hi there" {
		t.Fatalf("text = %q", result.Text)
	}
	if len(result.Voices) != 1 || result.Voices[0] != "Alice" {
		t.Fatalf("voices = %#v", result.Voices)
	}
}

func TestRenderTimestampsAndUnknownMarkup(t *testing.T) {
	nodes := ParseCueMarkup(`<00:00:01.000>one <x-note keep>two`)
	if got := RenderText(nodes, RenderOptions{}); got != "one <x-note keep>two" {
		t.Fatalf("default render = %q", got)
	}
	if got := RenderText(nodes, RenderOptions{KeepTimestamps: true, OmitUnknown: true}); got != "<00:00:01.000>one two" {
		t.Fatalf("explicit markup render = %q", got)
	}
}

func TestLanguageMatch(t *testing.T) {
	tests := []struct {
		preferred string
		actual    string
		want      bool
	}{
		{"en-us", "en-US", true},
		{"en", "en-US", true},
		{"en-US", "en", true},
		{"zh-Hant-TW", "zh-Hant", true},
		{"zh-Hant-TW", "zh-Hans", false},
		{"en", "fr", false},
		{"", "", true},
		{"", "en", false},
	}
	for _, test := range tests {
		if got := LanguageMatch(test.preferred, test.actual); got != test.want {
			t.Errorf("LanguageMatch(%q, %q) = %v, want %v",
				test.preferred, test.actual, got, test.want)
		}
	}
}

func TestCueTextUsesLanguageFallback(t *testing.T) {
	cue := Cue{
		Payload: `<v Maria><lang es>Hola</lang><lang en>Hello</lang></v>`,
	}
	if got := cue.Text("en"); got != "Hello" {
		t.Fatalf("cue.Text(en) = %q", got)
	}
	if got := cue.Text("es-MX"); got != "Hola" {
		t.Fatalf("cue.Text(es-MX) = %q", got)
	}
}

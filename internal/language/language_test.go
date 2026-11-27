package language

import "testing"

func TestDetect(t *testing.T) {
	cases := map[string]string{
		"你好，世界":                  Chinese,
		"Hello world":            English,
		"こんにちは世界":                Japanese,
		"日本語のテキスト":               Japanese,
		"안녕하세요":                  Korean,
		"Привет мир":             Russian,
		"مرحبا بالعالم":          Arabic,
		"123 ...":                Unknown,
		"中文 mixed English words": Chinese,
	}
	for text, want := range cases {
		if got := Detect(text); got != want {
			t.Errorf("Detect(%q) = %q, want %q", text, got, want)
		}
	}
}

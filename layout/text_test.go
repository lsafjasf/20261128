package layout

import (
	"strings"
	"testing"
)

func TestGraphemesKeepCombiningAndEmojiSequences(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"漢字", []string{"漢", "字"}},
		{"か\u3099", []string{"か\u3099"}},
		{"👩\u200d💻", []string{"👩\u200d💻"}},
		{"A👩\u200d💻B", []string{"A", "👩\u200d💻", "B"}},
		{"🇨🇳🇯🇵", []string{"🇨🇳", "🇯🇵"}},
		{"1\ufe0f\u20e3", []string{"1\ufe0f\u20e3"}},
	}
	for _, test := range cases {
		got := graphemes(test.input)
		if len(got) != len(test.want) {
			t.Fatalf("graphemes(%q) = %q, want %q", test.input, got, test.want)
		}
		for i := range got {
			if got[i] != test.want[i] {
				t.Fatalf("graphemes(%q)[%d] = %q, want %q", test.input, i, got[i], test.want[i])
			}
		}
	}
}

func TestDistributeClustersIsLossless(t *testing.T) {
	weights := []float64{2, 1, 3, 1}
	text := "かんじにほんご"
	parts := distributeClusters(text, weights)
	if got := strings.Join(parts, ""); got != text {
		t.Fatalf("distributed text = %q, want %q", got, text)
	}
	for _, part := range parts {
		if part == "" {
			t.Errorf("part should be non-empty when annotations >= clusters: %q", parts)
		}
	}
}

func TestSplitTextRunUsesCJKBreakUnitsAndWords(t *testing.T) {
	items := splitTextRun("Hello 世界")
	if len(items) != 4 {
		t.Fatalf("items = %+v, want word, space, CJK, CJK", items)
	}
	got := []string{items[0].text, items[1].text, items[2].text, items[3].text}
	want := []string{"Hello", " ", "世", "界"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %q, want %q", i, got[i], want[i])
		}
	}
}

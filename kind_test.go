package releaseaudit

import "testing"

func TestClassifyKindUsesWholeTokensAndAliases(t *testing.T) {
	tests := []struct {
		input string
		want  Kind
		ok    bool
	}{
		{"feature_mobile_v2", KindMobile, true},
		{"CINEMA_DCP", KindTheatrical, true},
		{"release_OTT_final", KindStreaming, true},
		{"影院版", KindTheatrical, true},
		{"流媒体版", KindStreaming, true},
		{"移动端", KindMobile, true},
		{"webster", "", false},
		{"theatrical_streaming", "", false},
		{"master_v1", "", false},
	}
	for _, test := range tests {
		got, ok := ClassifyKind(test.input)
		if got != test.want || ok != test.ok {
			t.Errorf("ClassifyKind(%q) = %q, %v; want %q, %v",
				test.input, got, ok, test.want, test.ok)
		}
	}
}

func TestAspectRatioAndLayoutParsing(t *testing.T) {
	for _, test := range []struct {
		input string
		want  float64
	}{
		{"2.39:1", 2.39},
		{"16/9", 16.0 / 9.0},
		{"1.85", 1.85},
	} {
		got, err := parseAspectRatio(test.input)
		if err != nil || got != test.want {
			t.Errorf("parseAspectRatio(%q) = %v, %v; want %v", test.input, got, err, test.want)
		}
	}
	if _, err := parseAspectRatio("0:1"); err == nil {
		t.Error("zero aspect ratio was accepted")
	}
	if got, ok := layoutChannels("5.1"); !ok || got != 6 {
		t.Errorf("layoutChannels(5.1) = %d, %v", got, ok)
	}
}

func TestPathKindsReportsEveryVersion(t *testing.T) {
	got := pathKinds("theatrical/copy_for_mobile.mxf")
	if len(got) != 2 {
		t.Fatalf("pathKinds = %v; want two kinds", got)
	}
}

package channelmap

import "testing"

func TestNormalizeLabelAliases(t *testing.T) {
	tests := map[string]Label{
		"FL":            LabelLeft,
		" front-left ":  LabelLeft,
		"FR":            LabelRight,
		"CENTER":        LabelCenter,
		"low frequency": LabelLFE,
		"BL":            LabelLeftRearSurround,
		"left_surround": LabelLeftSurround,
		"RRS":           LabelRightRearSurround,
		"back center":   LabelCenterSurround,
		"CHANNEL_12":    Label("CH12"),
		"ch 3":          Label("CH3"),
	}
	for input, want := range tests {
		got, err := NormalizeLabel(input)
		if err != nil {
			t.Errorf("NormalizeLabel(%q) error = %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeLabel(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestNormalizeLabelRejectsUnknown(t *testing.T) {
	if _, err := NormalizeLabel("front-middle-extra"); err == nil {
		t.Fatal("NormalizeLabel() accepted an unknown channel")
	}
}

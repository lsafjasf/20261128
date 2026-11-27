package channelmap

import (
	"fmt"
	"regexp"
	"strings"
)

// Label is the canonical name of a logical audio channel.
type Label string

const (
	LabelLeft              Label = "L"
	LabelRight             Label = "R"
	LabelCenter            Label = "C"
	LabelLFE               Label = "LFE"
	LabelLeftSurround      Label = "Ls"
	LabelRightSurround     Label = "Rs"
	LabelLeftRearSurround  Label = "Lrs"
	LabelRightRearSurround Label = "Rrs"
	LabelCenterSurround    Label = "Cs"
	LabelLeftOfCenter      Label = "Lc"
	LabelRightOfCenter     Label = "Rc"
	LabelTopCenter         Label = "Tc"
	LabelTopFrontLeft      Label = "Tfl"
	LabelTopFrontCenter    Label = "Tfc"
	LabelTopFrontRight     Label = "Tfr"
	LabelTopBackLeft       Label = "Tbl"
	LabelTopBackCenter     Label = "Tbc"
	LabelTopBackRight      Label = "Tbr"
)

var genericChannelPattern = regexp.MustCompile(`^CH(?:ANNEL)?_?([1-9][0-9]*)$`)

var labelAliases = map[string]Label{
	"L": LabelLeft, "FL": LabelLeft, "LEFT": LabelLeft, "FRONTLEFT": LabelLeft,
	"FRONT_LEFT": LabelLeft, "LEFT_FRONT": LabelLeft,

	"R": LabelRight, "FR": LabelRight, "RIGHT": LabelRight, "FRONTRIGHT": LabelRight,
	"FRONT_RIGHT": LabelRight, "RIGHT_FRONT": LabelRight,

	"C": LabelCenter, "FC": LabelCenter, "CNT": LabelCenter, "CENTER": LabelCenter,
	"FRONTCENTER": LabelCenter, "FRONT_CENTER": LabelCenter, "MIDDLE": LabelCenter,

	"LFE": LabelLFE, "LFE1": LabelLFE, "LOWFREQUENCY": LabelLFE,
	"LOW_FREQUENCY": LabelLFE, "SUB": LabelLFE, "SUBWOOFER": LabelLFE,

	"LS": LabelLeftSurround, "SL": LabelLeftSurround, "LEFT_SURROUND": LabelLeftSurround,
	"LEFT_SIDE": LabelLeftSurround, "SIDE_LEFT": LabelLeftSurround,
	"LSS": LabelLeftSurround, "LSD": LabelLeftSurround,

	"RS": LabelRightSurround, "SR": LabelRightSurround, "RIGHT_SURROUND": LabelRightSurround,
	"RIGHT_SIDE": LabelRightSurround, "SIDE_RIGHT": LabelRightSurround,
	"RSS": LabelRightSurround, "RSA": LabelRightSurround,

	"LRS": LabelLeftRearSurround, "BL": LabelLeftRearSurround, "LB": LabelLeftRearSurround,
	"BACK_LEFT": LabelLeftRearSurround, "REAR_LEFT": LabelLeftRearSurround,
	"LEFT_BACK": LabelLeftRearSurround, "LEFT_REAR": LabelLeftRearSurround,

	"RRS": LabelRightRearSurround, "BR": LabelRightRearSurround, "RB": LabelRightRearSurround,
	"BACK_RIGHT": LabelRightRearSurround, "REAR_RIGHT": LabelRightRearSurround,
	"RIGHT_BACK": LabelRightRearSurround, "RIGHT_REAR": LabelRightRearSurround,

	"CS": LabelCenterSurround, "BC": LabelCenterSurround, "CB": LabelCenterSurround,
	"BACK_CENTER": LabelCenterSurround, "REAR_CENTER": LabelCenterSurround,
	"CENTER_SURROUND": LabelCenterSurround,

	"LC": LabelLeftOfCenter, "FLC": LabelLeftOfCenter, "LEFT_OF_CENTER": LabelLeftOfCenter,
	"FRONT_LEFT_OF_CENTER": LabelLeftOfCenter,
	"RC":                   LabelRightOfCenter, "FRC": LabelRightOfCenter, "RIGHT_OF_CENTER": LabelRightOfCenter,
	"FRONT_RIGHT_OF_CENTER": LabelRightOfCenter,

	"TC": LabelTopCenter, "TOP_CENTER": LabelTopCenter,
	"TFL": LabelTopFrontLeft, "TOP_FRONT_LEFT": LabelTopFrontLeft,
	"TFC": LabelTopFrontCenter, "TOP_FRONT_CENTER": LabelTopFrontCenter,
	"TFR": LabelTopFrontRight, "TOP_FRONT_RIGHT": LabelTopFrontRight,
	"TBL": LabelTopBackLeft, "TOP_BACK_LEFT": LabelTopBackLeft,
	"TBC": LabelTopBackCenter, "TOP_BACK_CENTER": LabelTopBackCenter,
	"TBR": LabelTopBackRight, "TOP_BACK_RIGHT": LabelTopBackRight,
}

var labelNames = map[Label]string{
	LabelLeft:              "front left",
	LabelRight:             "front right",
	LabelCenter:            "front center",
	LabelLFE:               "low frequency effects",
	LabelLeftSurround:      "left surround",
	LabelRightSurround:     "right surround",
	LabelLeftRearSurround:  "left rear surround",
	LabelRightRearSurround: "right rear surround",
	LabelCenterSurround:    "center surround",
	LabelLeftOfCenter:      "front left of center",
	LabelRightOfCenter:     "front right of center",
	LabelTopCenter:         "top center",
	LabelTopFrontLeft:      "top front left",
	LabelTopFrontCenter:    "top front center",
	LabelTopFrontRight:     "top front right",
	LabelTopBackLeft:       "top back left",
	LabelTopBackCenter:     "top back center",
	LabelTopBackRight:      "top back right",
}

// NormalizeLabel maps common WAV, container, and broadcast aliases to one
// canonical label. Matching is case-insensitive and accepts spaces, dashes,
// dots, and slash separators in place of underscores.
func NormalizeLabel(raw string) (Label, error) {
	original := strings.TrimSpace(raw)
	if original == "" {
		return "", fmt.Errorf("empty channel label")
	}

	token := strings.ToUpper(original)
	token = strings.NewReplacer(" ", "_", "-", "_", ".", "_", "/", "_").Replace(token)
	token = strings.Join(strings.FieldsFunc(token, func(r rune) bool {
		return r == '_'
	}), "_")

	if label, ok := labelAliases[token]; ok {
		return label, nil
	}
	if match := genericChannelPattern.FindStringSubmatch(token); match != nil {
		return Label("CH" + match[1]), nil
	}
	return "", fmt.Errorf("unknown channel label %q", original)
}

// Name returns a human-readable description of a canonical label.
func (l Label) Name() string {
	if name, ok := labelNames[l]; ok {
		return name
	}
	if match := genericChannelPattern.FindStringSubmatch(string(l)); match != nil {
		return "generic channel " + match[1]
	}
	return string(l)
}

// Equivalent reports whether two labels refer to the same logical channel.
// It is intentionally exact after normalization; accepting "close enough"
// labels can hide a real C/Ls routing error.
func (l Label) Equivalent(other Label) bool {
	return l == other
}

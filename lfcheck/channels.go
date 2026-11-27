package lfcheck

import (
	"fmt"
	"strings"
)

// Roles contains zero-based channel indexes for the four channels used by the
// low-frequency audit.
type Roles struct {
	Left   int `json:"left"`
	Right  int `json:"right"`
	Center int `json:"center"`
	LFE    int `json:"lfe"`
}

// ChannelNames returns the encoded channel order. WAVE_FORMAT_EXTENSIBLE uses
// its channel mask; an unmasked six-channel file uses the conventional
// FL FR FC LFE BL BR order.
func ChannelNames(a Audio) ([]string, error) {
	if a.HasMask && a.ChannelMask != 0 {
		names, err := namesFromMask(a.ChannelMask, a.Channels)
		if err != nil {
			return nil, err
		}
		return names, nil
	}
	if a.Channels == 6 {
		return []string{"FL", "FR", "FC", "LFE", "BL", "BR"}, nil
	}
	if a.Channels < 4 {
		return nil, fmt.Errorf("%d channels is too few for a 5.1 audit; pass --layout", a.Channels)
	}
	names := make([]string, a.Channels)
	for i := range names {
		names[i] = fmt.Sprintf("CH%d", i+1)
	}
	return names, nil
}

// DefaultRoles resolves FL, FR, FC and LFE from the container layout.
func DefaultRoles(a Audio) (Roles, error) {
	names, err := ChannelNames(a)
	if err != nil {
		return Roles{}, err
	}
	return RolesFromLayout(names)
}

// RolesFromLayout maps a comma-separated encoded channel order to roles.
func RolesFromLayout(names []string) (Roles, error) {
	roles := Roles{Left: -1, Right: -1, Center: -1, LFE: -1}
	for i, raw := range names {
		switch canonicalName(raw) {
		case "L":
			if roles.Left >= 0 {
				return Roles{}, fmt.Errorf("layout names more than one left channel")
			}
			roles.Left = i
		case "R":
			if roles.Right >= 0 {
				return Roles{}, fmt.Errorf("layout names more than one right channel")
			}
			roles.Right = i
		case "C":
			if roles.Center >= 0 {
				return Roles{}, fmt.Errorf("layout names more than one center channel")
			}
			roles.Center = i
		case "LFE":
			if roles.LFE >= 0 {
				return Roles{}, fmt.Errorf("layout names more than one LFE channel")
			}
			roles.LFE = i
		}
	}
	if roles.Left < 0 || roles.Right < 0 || roles.Center < 0 || roles.LFE < 0 {
		return Roles{}, fmt.Errorf("layout must contain L, R, C and LFE")
	}
	return roles, nil
}

// ParseLayout parses labels separated by commas, spaces, or slashes.
func ParseLayout(spec string) ([]string, error) {
	fields := strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == ';' || r == '/' || r == ' ' || r == '\t'
	})
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty channel layout")
	}
	return fields, nil
}

func canonicalName(raw string) string {
	name := strings.ToUpper(strings.TrimSpace(raw))
	name = strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(name)
	switch name {
	case "FL", "LEFT", "FRONT_LEFT", "LEFT_FRONT":
		return "L"
	case "FR", "RIGHT", "FRONT_RIGHT", "RIGHT_FRONT":
		return "R"
	case "FC", "CENTER", "CENTRE", "FRONT_CENTER":
		return "C"
	case "LFE", "LFE1", "SUB", "SUBWOOFER", "LOW_FREQUENCY":
		return "LFE"
	}
	return name
}

func namesFromMask(mask uint32, channels int) ([]string, error) {
	labels := []string{
		"FL", "FR", "FC", "LFE", "BL", "BR", "FLC", "FRC",
		"BC", "SL", "SR", "TC", "TFL", "TFC", "TFR", "TBL",
		"TBC", "TBR",
	}
	names := make([]string, 0, channels)
	for bit, label := range labels {
		if mask&(1<<bit) != 0 {
			names = append(names, label)
		}
	}
	if len(names) != channels {
		return nil, fmt.Errorf("channel mask 0x%x describes %d channels, file has %d",
			mask, len(names), channels)
	}
	return names, nil
}

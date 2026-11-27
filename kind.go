package releaseaudit

import (
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var kindAliases = map[Kind][]string{
	KindTheatrical: {
		"theatrical", "theater", "theatre", "cinema", "cinematic", "dcp", "影院", "院线",
	},
	KindStreaming: {
		"streaming", "stream", "ott", "web", "svod", "流媒体", "网络",
	},
	KindMobile: {
		"mobile", "handheld", "phone", "smartphone", "移动端", "移动", "手机",
	},
}

// ClassifyKind maps a version ID, label, channel, or filename to a canonical
// distribution kind. English aliases are matched as whole tokens so that a
// substring such as "web" in "webster" is not treated as streaming.
func ClassifyKind(s string) (Kind, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}

	found, ok := classifyTokens(fileTokens(s))
	if ok {
		return found, true
	}
	lower := strings.ToLower(s)
	for _, kind := range DefaultKinds {
		for _, alias := range kindAliases[kind] {
			if containsNonASCII(alias) && strings.Contains(lower, alias) {
				return kind, true
			}
		}
	}
	return "", false
}

func classifyTokens(tokens []string) (Kind, bool) {
	var found Kind
	for _, token := range tokens {
		for _, kind := range DefaultKinds {
			for _, alias := range kindAliases[kind] {
				if token != alias {
					continue
				}
				if found != "" && found != kind {
					return "", false
				}
				found = kind
			}
		}
	}
	return found, found != ""
}

func containsNonASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return true
		}
	}
	return false
}

func fileTokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
}

// pathKinds returns every distinct version kind explicitly named in a path.
func pathKinds(path string) []Kind {
	seen := map[Kind]bool{}
	var kinds []Kind
	for _, token := range fileTokens(filepath.ToSlash(path)) {
		for _, kind := range DefaultKinds {
			for _, alias := range kindAliases[kind] {
				if token == alias && !seen[kind] {
					seen[kind] = true
					kinds = append(kinds, kind)
				}
			}
		}
	}
	for _, kind := range DefaultKinds {
		for _, alias := range kindAliases[kind] {
			if containsNonASCII(alias) && strings.Contains(strings.ToLower(path), alias) && !seen[kind] {
				seen[kind] = true
				kinds = append(kinds, kind)
			}
		}
	}
	return kinds
}

func (v Version) kind() (Kind, bool) {
	candidates := []string{v.ID, v.Window.Channel, v.Label}
	for _, candidate := range candidates {
		if kind, ok := ClassifyKind(candidate); ok {
			return kind, true
		}
	}
	return "", false
}

func versionName(v Version) string {
	if strings.TrimSpace(v.ID) != "" {
		return v.ID
	}
	if strings.TrimSpace(v.Label) != "" {
		return v.Label
	}
	return "(unnamed)"
}

func parseAspectRatio(s string) (float64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, strconv.ErrSyntax
	}
	s = strings.ReplaceAll(s, " ", "")
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ':' || r == '/'
	})
	if len(parts) == 1 {
		n, err := strconv.ParseFloat(parts[0], 64)
		if err != nil || n <= 0 || math.IsInf(n, 0) || math.IsNaN(n) {
			return 0, strconv.ErrSyntax
		}
		return n, nil
	}
	if len(parts) != 2 {
		return 0, strconv.ErrSyntax
	}
	n, err1 := strconv.ParseFloat(parts[0], 64)
	d, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || n <= 0 || d <= 0 || math.IsInf(n, 0) || math.IsInf(d, 0) {
		return 0, strconv.ErrSyntax
	}
	return n / d, nil
}

func layoutChannels(layout string) (int, bool) {
	layout = strings.ToLower(strings.TrimSpace(layout))
	switch layout {
	case "mono", "1.0":
		return 1, true
	case "stereo", "2.0":
		return 2, true
	case "2.1":
		return 3, true
	case "5.1":
		return 6, true
	case "7.1":
		return 8, true
	case "5.1.4":
		return 10, true
	case "7.1.4":
		return 12, true
	}
	return 0, false
}

func parseWindowTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

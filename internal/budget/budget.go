// Package budget parses LTO capacity budget strings such as "12TB".
package budget

import (
	"errors"
	"strconv"
	"strings"
)

// unit factors are decimal SI units, matching drive vendor LTO labels.
var units = map[string]int64{
	"":   1,
	"B":  1,
	"KB": 1_000,
	"MB": 1_000_000,
	"GB": 1_000_000_000,
	"TB": 1_000_000_000_000,
	"PB": 1_000_000_000_000_000,
}

// Parse converts a case-insensitive size string ("9TB", "500GB", "1024") to bytes.
func Parse(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, errors.New("empty budget")
	}
	i := 0
	for i < len(s) && (strings.ContainsRune("0123456789.", rune(s[i]))) {
		i++
	}
	numPart, unitPart := s[:i], strings.TrimSpace(s[i:])
	factor, ok := units[unitPart]
	if !ok {
		return 0, errors.New("unknown unit in budget: " + s)
	}
	value, err := strconv.ParseFloat(numPart, 64)
	if err != nil || value < 0 {
		return 0, errors.New("invalid budget value: " + s)
	}
	return int64(value * float64(factor)), nil
}

// Format renders bytes with a decimal unit for human-readable reports.
func Format(n int64) string {
	if n < 1_000 {
		return strconv.FormatInt(n, 10) + "B"
	}
	value := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB", "TB", "PB"} {
		value /= 1_000
		if value < 1_000 {
			return strconv.FormatFloat(value, 'f', 2, 64) + suffix
		}
	}
	return strconv.FormatFloat(value, 'f', 2, 64) + "PB"
}

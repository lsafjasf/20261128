package batchout

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxNameBytes = 255

// targetCandidates returns a stable conflict sequence:
//
//	report.mp4
//	report.<task-prefix>.mp4
//	report.<task-prefix>-2.mp4
//
// A task's result therefore remains recognizable without ever replacing an
// existing final, partial, or manually created file.
func targetCandidates(desired, taskID string) []string {
	desired = path.Clean(filepath.ToSlash(desired))
	dir, base := path.Split(desired)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = base
		ext = ""
	}
	short := taskID
	if len(short) > 12 {
		short = short[:12]
	}

	firstSuffix := "." + short
	output := []string{desired}
	output = append(output, dir+fitFilename(stem, firstSuffix, ext))
	for n := 2; n < 10000; n++ {
		output = append(output, dir+fitFilename(stem, fmt.Sprintf(".%s-%d", short, n), ext))
	}
	return output
}

func fitFilename(stem, suffix, ext string) string {
	limit := maxNameBytes - len(suffix) - len(ext)
	if limit < 1 {
		limit = 1
	}
	if len(stem) <= limit {
		return stem + suffix + ext
	}
	var b strings.Builder
	for _, r := range stem {
		next := b.Len() + utf8.RuneLen(r)
		if next > limit {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + suffix + ext
}

func existsNoFollow(path string) (bool, error) {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

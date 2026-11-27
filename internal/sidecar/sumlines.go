package sidecar

import (
	"bufio"
	"fmt"
	"path/filepath"
	"strings"

	"sidecaraudit/internal/algo"
)

// parseSumLines 解析 GNU sum 行格式。
func parseSumLines(name, content string) ([]Claim, error) {
	extAlgo, _ := algoFromFilename(name)
	var claims []Claim
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := sc.Text()
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		digest, path, ok := splitSumLine(line)
		if !ok {
			return nil, fmt.Errorf("%s:%d: 无法解析的行: %q", name, lineNo, line)
		}
		alg, known := extAlgo, true
		if extAlgo.Name == "" {
			alg, known = algo.ByHexLen(len(digest))
		}
		claims = append(claims, newClaim(name, lineNo, path, alg, digest, !known, raw))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return claims, nil
}

// splitSumLine 拆分 "<hex>  <path>" / "<hex> *<path>"。
func splitSumLine(line string) (digest, path string, ok bool) {
	i := strings.IndexAny(line, " \t")
	if i <= 0 {
		return "", "", false
	}
	digest = strings.ToLower(line[:i])
	rest := strings.TrimLeft(line[i:], " \t")
	rest = strings.TrimPrefix(rest, "*") // 二进制模式标记
	if rest == "" {
		return "", "", false
	}
	return digest, rest, true
}

// algoFromFilename 从扩展名推断算法,如 "release.sha256"、"SHA256SUMS"。
func algoFromFilename(name string) (algo.Algorithm, bool) {
	base := strings.ToLower(filepath.Base(name))
	base = strings.TrimSuffix(base, "sums")
	base = strings.TrimSuffix(base, "sum")
	ext := strings.TrimPrefix(filepath.Ext(base), ".")
	if ext == "" {
		ext = base
	}
	return algo.Lookup(ext)
}

func newClaim(sidecar string, line int, path string, a algo.Algorithm, digest string, unknown bool, raw string) Claim {
	name := a.Name
	if unknown {
		name = ""
	}
	return Claim{
		Sidecar:   sidecar,
		Line:      line,
		Path:      NormalizePath(path),
		Algorithm: name,
		Digest:    strings.ToLower(digest),
		Unknown:   unknown,
		Raw:       raw,
	}
}

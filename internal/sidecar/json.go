package sidecar

import (
	"encoding/json"
	"fmt"
	"strings"

	"sidecaraudit/internal/algo"
)

// jsonEntry 是 JSON 清单中的单条记录,支持两种写法:
// {"path": "...", "algorithm": "sha256", "digest": "..."}
// {"path": "...", "sha256": "..."}  (算法名作键)
type jsonEntry map[string]json.RawMessage

// parseJSON 解析 JSON 清单(顶层数组或 {"files": [...]})。
func parseJSON(name string, data []byte) ([]Claim, error) {
	var entries []jsonEntry
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		entries = make([]jsonEntry, 0, len(arr))
		for i, raw := range arr {
			var e jsonEntry
			if err := json.Unmarshal(raw, &e); err != nil {
				return nil, fmt.Errorf("%s: 第 %d 条不是对象: %w", name, i+1, err)
			}
			entries = append(entries, e)
		}
	} else {
		var obj struct {
			Files []jsonEntry `json:"files"`
		}
		if err := json.Unmarshal(data, &obj); err != nil || obj.Files == nil {
			return nil, fmt.Errorf("%s: 无法识别的 JSON 清单结构", name)
		}
		entries = obj.Files
	}

	var claims []Claim
	for i, e := range entries {
		cs, err := entryClaims(name, i+1, e)
		if err != nil {
			return nil, err
		}
		claims = append(claims, cs...)
	}
	return claims, nil
}

func entryClaims(sidecar string, line int, e jsonEntry) ([]Claim, error) {
	path, err := entryPath(e)
	if err != nil {
		return nil, fmt.Errorf("%s: 第 %d 条: %w", sidecar, line, err)
	}
	raw, _ := json.Marshal(e)

	if _, ok := e["algorithm"]; ok {
		var algName, digest string
		if err := json.Unmarshal(e["algorithm"], &algName); err != nil {
			return nil, fmt.Errorf("%s: 第 %d 条: algorithm 不是字符串", sidecar, line)
		}
		if err := json.Unmarshal(e["digest"], &digest); err != nil {
			return nil, fmt.Errorf("%s: 第 %d 条: 缺少 digest 字段", sidecar, line)
		}
		a, known := algo.Lookup(algName)
		c := newClaim(sidecar, line, path, a, digest, !known, string(raw))
		if !known {
			c.Algorithm = strings.ToLower(strings.TrimSpace(algName))
		}
		return []Claim{c}, nil
	}

	var claims []Claim
	for key, rawVal := range e {
		if key == "path" || key == "file" || key == "name" {
			continue
		}
		var digest string
		if err := json.Unmarshal(rawVal, &digest); err != nil {
			continue
		}
		a, known := algo.Lookup(key)
		c := newClaim(sidecar, line, path, a, digest, !known, string(raw))
		if !known {
			c.Algorithm = strings.ToLower(key)
		}
		claims = append(claims, c)
	}
	if len(claims) == 0 {
		return nil, fmt.Errorf("%s: 第 %d 条: 没有任何摘要字段", sidecar, line)
	}
	return claims, nil
}

func entryPath(e jsonEntry) (string, error) {
	for _, key := range []string{"path", "file", "name"} {
		if raw, ok := e[key]; ok {
			var p string
			if err := json.Unmarshal(raw, &p); err != nil || p == "" {
				return "", fmt.Errorf("%q 字段不是非空字符串", key)
			}
			return p, nil
		}
	}
	return "", fmt.Errorf("缺少 path/file/name 字段")
}

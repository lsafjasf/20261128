// Package sidecar 解析 checksum sidecar 文件,产出带出处信息的断言(Claim)。
//
// 支持两种格式:
//   - GNU coreutils sum 行格式: "<hex>  <path>" 或 "<hex> *<path>",
//     算法由文件扩展名(.sha256/.md5/...)或摘要长度推断;
//   - JSON 清单: [{"path","algorithm","digest"}, ...] 或 {"files": [...]},
//     条目也可用算法名作键,如 {"path": "a.bin", "sha256": "..."}。
package sidecar

import (
	"os"
	"path/filepath"
	"strings"
)

// Claim 是 sidecar 中一条 "某路径的某算法摘要为 X" 的断言,携带来源证据。
type Claim struct {
	Sidecar   string `json:"sidecar"`      // sidecar 文件路径
	Line      int    `json:"line"`         // 行号(JSON 条目为数组下标+1)
	Path      string `json:"path"`         // 规范化后的目标文件路径
	Algorithm string `json:"algorithm"`    // 规范化算法名;未知算法保留原文
	Digest    string `json:"digest"`       // 小写十六进制摘要
	Unknown   bool   `json:"unknown_algo"` // 算法未知,降级为不可判定
	Raw       string `json:"raw"`          // 原始行/条目,供证据链追溯
}

// ParseFile 解析单个 sidecar 文件,自动识别格式。
func ParseFile(name string) ([]Claim, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimLeft(string(data), " \t\r\n")
	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
		return parseJSON(name, data)
	}
	return parseSumLines(name, string(data))
}

// NormalizePath 统一路径形态,作为文件身份的键。
func NormalizePath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	p = strings.TrimPrefix(p, "./")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(p)), "./")
}

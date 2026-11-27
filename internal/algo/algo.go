// Package algo 维护受支持的摘要算法注册表。
// 未注册的算法一律视为未知,对应条目降级为"不可判定"。
package algo

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"strings"
)

// Algorithm 描述一个受支持的摘要算法。
type Algorithm struct {
	Name    string // 规范化名称,如 "sha256"
	HexLen  int    // 十六进制摘要的字符数
	NewHash func() hash.Hash
}

var registry = map[string]Algorithm{}

func register(a Algorithm, aliases ...string) {
	registry[a.Name] = a
	for _, alias := range aliases {
		registry[alias] = a
	}
}

func init() {
	register(Algorithm{Name: "md5", HexLen: 32, NewHash: md5.New})
	register(Algorithm{Name: "sha1", HexLen: 40, NewHash: sha1.New}, "sha-1")
	register(Algorithm{Name: "sha224", HexLen: 56, NewHash: sha256.New224}, "sha-224")
	register(Algorithm{Name: "sha256", HexLen: 64, NewHash: sha256.New}, "sha-256")
	register(Algorithm{Name: "sha384", HexLen: 96, NewHash: sha512.New384}, "sha-384")
	register(Algorithm{Name: "sha512", HexLen: 128, NewHash: sha512.New}, "sha-512")
}

// Lookup 按名称(大小写不敏感)查找算法;未知算法返回 false。
func Lookup(name string) (Algorithm, bool) {
	a, ok := registry[strings.ToLower(strings.TrimSpace(name))]
	return a, ok
}

// ByHexLen 按十六进制摘要长度推断算法(用于无扩展名的 sum 文件)。
// 长度无法唯一确定或不受支持时返回 false。
func ByHexLen(n int) (Algorithm, bool) {
	for _, name := range []string{"md5", "sha1", "sha224", "sha256", "sha384", "sha512"} {
		if registry[name].HexLen == n {
			return registry[name], true
		}
	}
	return Algorithm{}, false
}

// ValidDigest 校验摘要是否为指定算法的合法十六进制串。
func ValidDigest(a Algorithm, digest string) bool {
	if len(digest) != a.HexLen {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

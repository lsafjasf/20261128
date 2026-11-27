package renderpipe

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
)

func writeField(h hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(value))
}

func joinHash(domain string, fields ...string) string {
	h := sha256.New()
	writeField(h, domain)
	for _, field := range fields {
		writeField(h, field)
	}
	return hexSum(h.Sum(nil))
}

func hexSum(sum []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(sum)*2)
	for i, b := range sum {
		out[i*2] = digits[b>>4]
		out[i*2+1] = digits[b&0x0f]
	}
	return string(out)
}

// Package hashfile computes streaming SHA-256 digests of regular files.
package hashfile

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

const bufferSize = 1 << 20 // 1 MiB keeps large video files memory-bounded

// SHA256 returns the lower-case hex SHA-256 of the file at path.
func SHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	digest := sha256.New()
	buf := make([]byte, bufferSize)
	if _, err := io.CopyBuffer(digest, f, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

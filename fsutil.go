package batchout

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func writeJSONAtomic(path string, value any, perm os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return syncDir(dir)
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func randomID() (string, error) {
	var buf [8]byte
	if _, err := io.ReadFull(rand.Reader, buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// cleanRelative validates producer-controlled and caller-controlled paths. It
// rejects absolute paths, parent traversal, NULs, and the internal state root.
func cleanRelative(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, 0) {
		return "", errors.New("empty or invalid relative path")
	}
	if !utf8.ValidString(name) {
		return "", errors.New("relative path is not valid UTF-8")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute path is not allowed: %q", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes output directory: %q", name)
	}
	if clean == stateRoot || strings.HasPrefix(clean, stateRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path is reserved: %q", name)
	}
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute path is not allowed: %q", name)
	}
	return filepath.ToSlash(clean), nil
}

func nativeRel(rel string) string {
	return filepath.FromSlash(rel)
}

func joinBelow(root, rel string) (string, error) {
	clean, err := cleanRelative(rel)
	if err != nil {
		return "", err
	}
	full := filepath.Join(root, nativeRel(clean))
	relative, err := filepath.Rel(root, full)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root: %q", rel)
	}
	return full, nil
}

// ensureDirPath creates parents without following symlinks. Existing
// non-directories and symlinks are refused so a producer cannot redirect a
// transaction into a manually managed path.
func ensureDirPath(root, rel string) error {
	current := root
	if rel == "." || rel == "" {
		return nil
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid directory path: %q", rel)
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		switch {
		case err == nil:
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("%s exists and is not a real directory", current)
			}
		case errors.Is(err, os.ErrNotExist):
			if mkdirErr := os.Mkdir(current, 0o755); mkdirErr != nil {
				if !errors.Is(mkdirErr, os.ErrExist) {
					return mkdirErr
				}
				info, statErr := os.Lstat(current)
				if statErr != nil {
					return statErr
				}
				if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
					return fmt.Errorf("%s exists and is not a real directory", current)
				}
			}
			if err := syncDir(filepath.Dir(current)); err != nil {
				return err
			}
		default:
			return err
		}
	}
	return nil
}

func fileDigest(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(h.Sum(nil)), nil
}

package rendercache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var ErrCacheEntryCorrupt = errors.New("cache entry corrupt")

// Store persists stage output by cache key. Implementations must treat a Get
// miss distinctly from an I/O or integrity failure.
type Store interface {
	Get(ctx context.Context, key string) (data []byte, found bool, err error)
	Put(ctx context.Context, key string, data []byte) error
}

// MemoryStore is a concurrency-safe, process-local Store.
type MemoryStore struct {
	mu      sync.RWMutex
	entries map[string][]byte
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: make(map[string][]byte)}
}

func (s *MemoryStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	data, ok := s.entries[key]
	if ok {
		data = append([]byte(nil), data...)
	}
	s.mu.RUnlock()
	return data, ok, nil
}

func (s *MemoryStore) Put(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.entries == nil {
		s.entries = make(map[string][]byte)
	}
	s.entries[key] = append([]byte(nil), data...)
	s.mu.Unlock()
	return nil
}

var fileMagic = [8]byte{'R', 'C', 'A', 'C', 'H', 'E', '1', '\n'}

type fileMetadata struct {
	Key    string `json:"key"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

// FileStore stores each cache entry in one atomically replaced file. A
// separate metadata header lets Get reject truncated, mismatched, or
// corrupted payloads instead of serving a bad render.
type FileStore struct {
	root string
}

func NewFileStore(root string) (*FileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("cache root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve cache root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create cache root: %w", err)
	}
	return &FileStore{root: abs}, nil
}

func (s *FileStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	path, err := s.entryPath(key)
	if err != nil {
		return nil, false, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read cache entry: %w", err)
	}
	data, err := decodeEntry(key, raw)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (s *FileStore) Put(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.entryPath(key)
	if err != nil {
		return err
	}
	envelope, err := encodeEntry(key, data)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create cache shard: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".entry-*")
	if err != nil {
		return fmt.Errorf("create temporary cache entry: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(envelope); err != nil {
		tmp.Close()
		return fmt.Errorf("write cache entry: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync cache entry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close cache entry: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("publish cache entry: %w", err)
	}
	return nil
}

func (s *FileStore) entryPath(key string) (string, error) {
	hexKey, err := cacheKeyHex(key)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, hexKey[:2], hexKey+".entry"), nil
}

func cacheKeyHex(key string) (string, error) {
	value := strings.TrimPrefix(key, "sha256:")
	if len(value) != sha256.Size*2 {
		return "", fmt.Errorf("%w: cache key must be a SHA-256 value", ErrInvalidCacheKeyInput)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", fmt.Errorf("%w: cache key is not hexadecimal", ErrInvalidCacheKeyInput)
	}
	return strings.ToLower(value), nil
}

func encodeEntry(key string, data []byte) ([]byte, error) {
	if _, err := cacheKeyHex(key); err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(fileMetadata{
		Key:    key,
		Size:   int64(len(data)),
		Digest: outputDigest(data),
	})
	if err != nil {
		return nil, fmt.Errorf("encode cache metadata: %w", err)
	}
	var header [8]byte
	binary.BigEndian.PutUint64(header[:], uint64(len(metadata)))

	var out bytes.Buffer
	out.Grow(len(fileMagic) + len(header) + len(metadata) + len(data))
	out.Write(fileMagic[:])
	out.Write(header[:])
	out.Write(metadata)
	out.Write(data)
	return out.Bytes(), nil
}

func decodeEntry(key string, raw []byte) ([]byte, error) {
	headerSize := len(fileMagic) + 8
	if len(raw) < headerSize {
		return nil, fmt.Errorf("%w: truncated header", ErrCacheEntryCorrupt)
	}
	if !bytes.Equal(raw[:len(fileMagic)], fileMagic[:]) {
		return nil, fmt.Errorf("%w: bad magic", ErrCacheEntryCorrupt)
	}
	metadataSize := binary.BigEndian.Uint64(raw[len(fileMagic):headerSize])
	if metadataSize > 1<<20 || metadataSize > uint64(len(raw)-headerSize) {
		return nil, fmt.Errorf("%w: invalid metadata size", ErrCacheEntryCorrupt)
	}
	metadataEnd := headerSize + int(metadataSize)
	var metadata fileMetadata
	if err := json.Unmarshal(raw[headerSize:metadataEnd], &metadata); err != nil {
		return nil, fmt.Errorf("%w: metadata: %v", ErrCacheEntryCorrupt, err)
	}
	data := raw[metadataEnd:]
	if metadata.Key != key {
		return nil, fmt.Errorf("%w: key mismatch", ErrCacheEntryCorrupt)
	}
	if metadata.Size != int64(len(data)) {
		return nil, fmt.Errorf("%w: size mismatch", ErrCacheEntryCorrupt)
	}
	if metadata.Digest != outputDigest(data) {
		return nil, fmt.Errorf("%w: payload digest mismatch", ErrCacheEntryCorrupt)
	}
	return append([]byte(nil), data...), nil
}

func outputDigest(data []byte) string {
	h := sha256.New()
	h.Write([]byte("rendercache/output/v1\x00"))
	h.Write(data)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

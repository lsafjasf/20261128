package rendercache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testCacheKey(t *testing.T) string {
	t.Helper()
	key, err := BuildStageKey("project-a", "color", "v1", []byte("config"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return key.Key
}

func TestFileStoreRoundTripAndPersistence(t *testing.T) {
	root := t.TempDir()
	key := testCacheKey(t)
	payload := []byte("rendered pixels")

	first, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Put(context.Background(), key, payload); err != nil {
		t.Fatal(err)
	}

	second, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := second.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("persisted entry was not found")
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

func TestFileStoreRejectsCorruptPayload(t *testing.T) {
	root := t.TempDir()
	key := testCacheKey(t)
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), key, []byte("valid output")); err != nil {
		t.Fatal(err)
	}

	files, err := filepath.Glob(filepath.Join(root, "*", "*.entry"))
	if err != nil || len(files) != 1 {
		t.Fatalf("cache files = %v, err = %v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xff
	if err := os.WriteFile(files[0], raw, 0o644); err != nil {
		t.Fatal(err)
	}

	_, found, err := store.Get(context.Background(), key)
	if found {
		t.Fatal("corrupt entry was reported as a hit")
	}
	if !errors.Is(err, ErrCacheEntryCorrupt) {
		t.Fatalf("Get error = %v, want ErrCacheEntryCorrupt", err)
	}
}

func TestFileStoreMiss(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := store.Get(context.Background(), testCacheKey(t))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("empty cache reported a hit")
	}
}

func TestMemoryStoreCopiesPayloads(t *testing.T) {
	store := NewMemoryStore()
	key := testCacheKey(t)
	payload := []byte("one")
	if err := store.Put(context.Background(), key, payload); err != nil {
		t.Fatal(err)
	}
	payload[0] = 'x'
	got, found, err := store.Get(context.Background(), key)
	if err != nil || !found {
		t.Fatalf("Get = %q, %v, %v", got, found, err)
	}
	if string(got) != "one" {
		t.Fatalf("stored payload changed to %q", got)
	}
	got[0] = 'y'
	got, _, _ = store.Get(context.Background(), key)
	if string(got) != "one" {
		t.Fatalf("returned payload aliases store: %q", got)
	}
}

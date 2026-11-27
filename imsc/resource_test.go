package imsc

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func shaHex(t *testing.T, data []byte) string {
	t.Helper()
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestRegistryDeduplicatesIdenticalContent(t *testing.T) {
	reg := NewRegistry()
	png := []byte("PNG-BYTES-ONE")

	r1, c1 := reg.RegisterContent("images/logo.png", KindExternal, "image/png", png, "")
	r2, c2 := reg.RegisterContent("images/logo-copy.png", KindExternal, "image/png", png, "")

	if c1 != nil || c2 != nil {
		t.Fatalf("unexpected conflicts: %v %v", c1, c2)
	}
	if r1.Key != r2.Key {
		t.Fatalf("identical content must share one key, got %q and %q", r1.Key, r2.Key)
	}
	if reg.Len() != 1 {
		t.Fatalf("expected 1 resource, got %d", reg.Len())
	}
	if len(r1.URIs) != 2 {
		t.Fatalf("expected both URIs recorded, got %v", r1.URIs)
	}
	got, ok := reg.Lookup("images/logo-copy.png")
	if !ok || got.Key != r1.Key {
		t.Fatalf("second URI must resolve to the shared resource")
	}
}

func TestRegistryBlocksMergeOnSameURIDifferentContent(t *testing.T) {
	reg := NewRegistry()
	first := []byte("PNG-BYTES-ORIGINAL")
	other := []byte("PNG-BYTES-TAMPERED")

	r1, _ := reg.RegisterContent("images/logo.png", KindExternal, "image/png", first, "")
	r2, conflict := reg.RegisterContent("images/logo.png", KindExternal, "image/png", other, "")

	if conflict == nil {
		t.Fatal("expected a URIConflict for same URI with different content")
	}
	if conflict.FirstKey != r1.Key || conflict.ConflictingKey != r2.Key {
		t.Fatalf("conflict keys mismatch: %+v", conflict)
	}
	if r1.Key == r2.Key {
		t.Fatal("different payloads must never be merged")
	}
	if reg.Len() != 2 {
		t.Fatalf("expected both payloads retained, got %d resources", reg.Len())
	}
	if !reg.HasConflict("images/logo.png") {
		t.Fatal("HasConflict must report the offending URI")
	}
	// First writer wins: the URI keeps binding to the original resource.
	bound, _ := reg.Lookup("images/logo.png")
	if bound.Key != r1.Key {
		t.Fatalf("URI binding must stay on first resource, got %q", bound.Key)
	}
	if len(reg.Conflicts()) != 1 {
		t.Fatalf("expected exactly one recorded conflict")
	}
}

func TestRegistryExternalResourcesAreNotHashMerged(t *testing.T) {
	reg := NewRegistry()
	r1 := reg.RegisterExternal("images/a.png")
	r2 := reg.RegisterExternal("images/b.png")
	if r1.Key == r2.Key {
		t.Fatal("distinct external URIs must not merge without known bytes")
	}
	if !strings.HasPrefix(r1.Key, externalPrefix) {
		t.Fatalf("external key must carry prefix, got %q", r1.Key)
	}
}

func TestRegistryFallbackChainAndCycle(t *testing.T) {
	reg := NewRegistry()
	primary := []byte("primary")
	reg.RegisterContent("hd/logo.png", KindExternal, "image/png", primary, "")

	reg.SetFallback("4k/logo.png", "hd/logo.png")
	reg.SetFallback("missing/logo.png", "4k/logo.png")

	got, err := reg.Resolve("missing/logo.png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hd/logo.png" {
		t.Fatalf("fallback should reach hd/logo.png, got %q", got)
	}
	chain := reg.Chain("missing/logo.png")
	if len(chain) != 3 || chain[2] != "hd/logo.png" {
		t.Fatalf("unexpected chain: %v", chain)
	}

	reg.SetFallback("a", "b")
	reg.SetFallback("b", "a")
	if _, err := reg.Resolve("a"); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestParseDataURIRoundTrip(t *testing.T) {
	payload := []byte{0x00, 0x01, 0x02, 'h', 'i'}
	encoded := EncodeDataURI("image/png", payload)
	mediaType, decoded, err := ParseDataURI(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if mediaType != "image/png" || string(decoded) != string(payload) {
		t.Fatalf("round trip mismatch: %q %v", mediaType, decoded)
	}
	if shaHex(t, decoded) == "" {
		t.Fatal("hash should be computable on decoded payload")
	}
}

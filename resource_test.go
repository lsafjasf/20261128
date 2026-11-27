package rendercache

import (
	"errors"
	"testing"
)

func testRef(project string, kind ResourceKind, name, version string) ResourceRef {
	return ResourceRef{Project: project, Kind: kind, Name: name, Version: version}
}

func TestFingerprintBytes(t *testing.T) {
	first := FingerprintBytes([]byte("font bytes"))
	if !first.Valid() {
		t.Fatalf("fingerprint %q is not canonical", first)
	}
	if second := FingerprintBytes([]byte("font bytes")); second != first {
		t.Fatalf("same bytes produced %q and %q", first, second)
	}
	if changed := FingerprintBytes([]byte("font bytez")); changed == first {
		t.Fatal("changed content did not change the fingerprint")
	}
}

func TestRegistryReplacingVersionChangesFingerprint(t *testing.T) {
	registry := NewRegistry()
	ref := testRef("project-a", KindLUT, "show.cube", "v1")

	first, err := registry.Put(Resource{Ref: ref, Data: []byte("lut one")})
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.Put(Resource{Ref: ref, Data: []byte("lut two")})
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint == second.Fingerprint {
		t.Fatal("replacing version bytes did not change the fingerprint")
	}

	snapshot, err := registry.SnapshotAll([]ResourceRef{ref})
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot[ref].VerifyFingerprint(); err != nil {
		t.Fatalf("snapshot does not match its fingerprint: %v", err)
	}
	if string(snapshot[ref].Data) != "lut two" {
		t.Fatalf("snapshot has %q", snapshot[ref].Data)
	}
}

func TestRegistryUnknownVersion(t *testing.T) {
	registry := NewRegistry()
	known := testRef("project-a", KindFont, "title.otf", "v1")
	if _, err := registry.Put(Resource{Ref: known, Data: []byte("font")}); err != nil {
		t.Fatal(err)
	}

	unknown := known
	unknown.Version = "v2"
	_, err := registry.Resolve(unknown)
	if !errors.Is(err, ErrUnknownResourceVersion) {
		t.Fatalf("Resolve error = %v, want ErrUnknownResourceVersion", err)
	}

	empty := known
	empty.Version = ""
	_, err = registry.Resolve(empty)
	if !errors.Is(err, ErrUnknownResourceVersion) {
		t.Fatalf("empty-version error = %v, want ErrUnknownResourceVersion", err)
	}
}

package rendercache

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ResourceKind identifies the role a resource plays in a render pipeline.
// The built-in kinds are common render inputs; other non-empty kinds are
// accepted so projects can add their own asset types.
type ResourceKind string

const (
	KindLUT   ResourceKind = "lut"
	KindFont  ResourceKind = "font"
	KindAudio ResourceKind = "audio"
)

var (
	ErrInvalidResourceRef     = errors.New("invalid resource reference")
	ErrUnknownResourceVersion = errors.New("unknown resource version")
	ErrFingerprintMismatch    = errors.New("resource fingerprint mismatch")
)

// ResourceRef names a versioned resource inside one project. Project is part
// of the identity: equal names in different projects never refer to the same
// cache input.
type ResourceRef struct {
	Project string       `json:"project"`
	Kind    ResourceKind `json:"kind"`
	Name    string       `json:"name"`
	Version string       `json:"version"`
}

func (r ResourceRef) validate() error {
	switch {
	case strings.TrimSpace(r.Project) == "":
		return fmt.Errorf("%w: project is empty", ErrInvalidResourceRef)
	case strings.TrimSpace(string(r.Kind)) == "":
		return fmt.Errorf("%w: kind is empty", ErrInvalidResourceRef)
	case strings.TrimSpace(r.Name) == "":
		return fmt.Errorf("%w: name is empty", ErrInvalidResourceRef)
	case strings.TrimSpace(r.Version) == "":
		return fmt.Errorf("%w: %s/%s/%s has an empty version", ErrUnknownResourceVersion, r.Project, r.Kind, r.Name)
	}
	return nil
}

// Fingerprint is a SHA-256 content digest in "sha256:<lower-case hex>" form.
type Fingerprint string

// FingerprintBytes returns a content fingerprint with a domain separator so
// raw render output and resource bytes cannot share an accidental digest
// namespace.
func FingerprintBytes(data []byte) Fingerprint {
	h := sha256.New()
	h.Write([]byte("rendercache/resource/v1\x00"))
	h.Write(data)
	return Fingerprint("sha256:" + hex.EncodeToString(h.Sum(nil)))
}

// Valid reports whether f has the canonical SHA-256 fingerprint form.
func (f Fingerprint) Valid() bool {
	const prefix = "sha256:"
	s := string(f)
	if !strings.HasPrefix(s, prefix) || len(s) != len(prefix)+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s[len(prefix):])
	return err == nil && strings.ToLower(s[len(prefix):]) == s[len(prefix):]
}

// Resource is an exact version of an input asset. Data is copied by the
// registry before it is indexed.
type Resource struct {
	Ref  ResourceRef
	Data []byte
}

// ResolvedResource is the immutable registry view of a resource version.
type ResolvedResource struct {
	Ref         ResourceRef
	Fingerprint Fingerprint
	Size        int64
}

// ResourceSnapshot is an immutable view of resource metadata and bytes.
// Runners receive snapshots so they consume exactly the content that was
// fingerprinted, even if the registry changes concurrently.
type ResourceSnapshot struct {
	ResolvedResource
	Data []byte
}

// VerifyFingerprint checks that the snapshot bytes still match the digest
// recorded for the resource version.
func (s ResourceSnapshot) VerifyFingerprint() error {
	if actual := FingerprintBytes(s.Data); actual != s.Fingerprint {
		return fmt.Errorf("%w: %s/%s/%s@%s: got %s, want %s",
			ErrFingerprintMismatch, s.Ref.Project, s.Ref.Kind, s.Ref.Name,
			s.Ref.Version, actual, s.Fingerprint)
	}
	return nil
}

// UnknownResourceError reports a version that the registry cannot resolve.
// Callers must treat it as a cache miss, never as a reason to reuse an older
// version of the same named resource.
type UnknownResourceError struct {
	Ref ResourceRef
}

func (e *UnknownResourceError) Error() string {
	return fmt.Sprintf("%v: %s/%s/%s@%s",
		ErrUnknownResourceVersion, e.Ref.Project, e.Ref.Kind, e.Ref.Name, e.Ref.Version)
}

func (e *UnknownResourceError) Unwrap() error {
	return ErrUnknownResourceVersion
}

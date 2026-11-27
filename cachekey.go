package rendercache

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const cacheKeySchema = "rendercache/stage-key/v1"

var ErrInvalidCacheKeyInput = errors.New("invalid cache key input")

// DependencyKey is the already-computed key of an upstream stage.
type DependencyKey struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// Manifest captures every input that contributed to a cache key. It can be
// stored alongside diagnostics so a render can be reproduced or audited.
type Manifest struct {
	Schema       string             `json:"schema"`
	Project      string             `json:"project"`
	Stage        string             `json:"stage"`
	StageVersion string             `json:"stage_version"`
	Config       Fingerprint        `json:"config_fingerprint"`
	Resources    []ManifestResource `json:"resources"`
	Dependencies []DependencyKey    `json:"dependencies"`
}

type ManifestResource struct {
	Ref         ResourceRef `json:"ref"`
	Fingerprint Fingerprint `json:"fingerprint"`
	Size        int64       `json:"size"`
}

// StageKey is a content-addressed cache identity plus the manifest that
// explains it.
type StageKey struct {
	Key      string
	Manifest Manifest
}

// BuildStageKey derives a project-scoped stage key from the stage recipe and
// all of its direct inputs. Sorting resource refs and dependency names makes
// the result independent of caller map/order iteration.
func BuildStageKey(
	project string,
	stage string,
	stageVersion string,
	config []byte,
	resources []ResolvedResource,
	dependencies map[string]string,
) (StageKey, error) {
	if strings.TrimSpace(project) == "" {
		return StageKey{}, fmt.Errorf("%w: project is empty", ErrInvalidCacheKeyInput)
	}
	if strings.TrimSpace(stage) == "" {
		return StageKey{}, fmt.Errorf("%w: stage is empty", ErrInvalidCacheKeyInput)
	}
	if strings.TrimSpace(stageVersion) == "" {
		return StageKey{}, fmt.Errorf("%w: stage version is empty", ErrInvalidCacheKeyInput)
	}

	resourceByRef := make(map[ResourceRef]ResolvedResource, len(resources))
	for _, resource := range resources {
		if err := resource.Ref.validate(); err != nil {
			return StageKey{}, fmt.Errorf("%w: %w", ErrInvalidCacheKeyInput, err)
		}
		if previous, ok := resourceByRef[resource.Ref]; ok && previous.Fingerprint != resource.Fingerprint {
			return StageKey{}, fmt.Errorf("%w: resource %s/%s/%s@%s has conflicting fingerprints",
				ErrInvalidCacheKeyInput, resource.Ref.Project, resource.Ref.Kind,
				resource.Ref.Name, resource.Ref.Version)
		}
		if !resource.Fingerprint.Valid() {
			return StageKey{}, fmt.Errorf("%w: resource %s/%s/%s@%s has invalid fingerprint %q",
				ErrInvalidCacheKeyInput, resource.Ref.Project, resource.Ref.Kind,
				resource.Ref.Name, resource.Ref.Version, resource.Fingerprint)
		}
		resourceByRef[resource.Ref] = resource
	}

	refs := make([]ResourceRef, 0, len(resourceByRef))
	for ref := range resourceByRef {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return refLess(refs[i], refs[j])
	})

	names := make([]string, 0, len(dependencies))
	for name, key := range dependencies {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(key) == "" {
			return StageKey{}, fmt.Errorf("%w: dependency name and key must be non-empty", ErrInvalidCacheKeyInput)
		}
		names = append(names, name)
	}
	sort.Strings(names)

	configFingerprint := digestFingerprint("rendercache/config/v1", config)
	h := sha256.New()
	w := keyWriter{hash: h}
	w.string(cacheKeySchema)
	w.string(project)
	w.string(stage)
	w.string(stageVersion)
	w.string(string(configFingerprint))
	w.uint64(uint64(len(refs)))
	manifestResources := make([]ManifestResource, 0, len(refs))
	for _, ref := range refs {
		resource := resourceByRef[ref]
		w.string(ref.Project)
		w.string(string(ref.Kind))
		w.string(ref.Name)
		w.string(ref.Version)
		w.string(string(resource.Fingerprint))
		manifestResources = append(manifestResources, ManifestResource{
			Ref:         ref,
			Fingerprint: resource.Fingerprint,
			Size:        resource.Size,
		})
	}
	w.uint64(uint64(len(names)))
	manifestDependencies := make([]DependencyKey, 0, len(names))
	for _, name := range names {
		w.string(name)
		w.string(dependencies[name])
		manifestDependencies = append(manifestDependencies, DependencyKey{
			Name: name,
			Key:  dependencies[name],
		})
	}

	return StageKey{
		Key: "sha256:" + hex.EncodeToString(h.Sum(nil)),
		Manifest: Manifest{
			Schema:       cacheKeySchema,
			Project:      project,
			Stage:        stage,
			StageVersion: stageVersion,
			Config:       configFingerprint,
			Resources:    manifestResources,
			Dependencies: manifestDependencies,
		},
	}, nil
}

func refLess(a, b ResourceRef) bool {
	if a.Project != b.Project {
		return a.Project < b.Project
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.Version < b.Version
}

func digestFingerprint(domain string, data []byte) Fingerprint {
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write([]byte{0})
	h.Write(data)
	return Fingerprint("sha256:" + hex.EncodeToString(h.Sum(nil)))
}

// keyWriter frames each field with its length to prevent ambiguous
// concatenations such as ("ab", "c") and ("a", "bc").
type keyWriter struct {
	hash interface {
		Write([]byte) (int, error)
	}
}

func (w keyWriter) string(value string) {
	w.bytes([]byte(value))
}

func (w keyWriter) bytes(value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = w.hash.Write(size[:])
	_, _ = w.hash.Write(value)
}

func (w keyWriter) uint64(value uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], value)
	_, _ = w.hash.Write(buf[:])
}

package rendercache

import (
	"errors"
	"testing"
)

func resolved(ref ResourceRef, data string) ResolvedResource {
	return ResolvedResource{
		Ref:         ref,
		Fingerprint: FingerprintBytes([]byte(data)),
		Size:        int64(len(data)),
	}
}

func TestBuildStageKeyIsOrderIndependent(t *testing.T) {
	lut := resolved(testRef("project-a", KindLUT, "show.cube", "v1"), "lut")
	font := resolved(testRef("project-a", KindFont, "title.otf", "v1"), "font")

	first, err := BuildStageKey("project-a", "composite", "v3", []byte(`{"width":1920}`),
		[]ResolvedResource{lut, font}, map[string]string{"color": "sha256:aaa", "text": "sha256:bbb"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildStageKey("project-a", "composite", "v3", []byte(`{"width":1920}`),
		[]ResolvedResource{font, lut}, map[string]string{"text": "sha256:bbb", "color": "sha256:aaa"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Key != second.Key {
		t.Fatalf("key depends on input order: %q != %q", first.Key, second.Key)
	}
}

func TestBuildStageKeyInvalidatesEachInput(t *testing.T) {
	lut := resolved(testRef("project-a", KindLUT, "show.cube", "v1"), "lut")
	base, err := BuildStageKey("project-a", "color", "v1", []byte("config-a"),
		[]ResolvedResource{lut}, map[string]string{"source": "sha256:source"})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		key  func() (StageKey, error)
	}{
		{
			name: "resource content",
			key: func() (StageKey, error) {
				return BuildStageKey("project-a", "color", "v1", []byte("config-a"),
					[]ResolvedResource{resolved(lut.Ref, "new lut")},
					map[string]string{"source": "sha256:source"})
			},
		},
		{
			name: "resource version",
			key: func() (StageKey, error) {
				other := lut
				other.Ref.Version = "v2"
				return BuildStageKey("project-a", "color", "v1", []byte("config-a"),
					[]ResolvedResource{other}, map[string]string{"source": "sha256:source"})
			},
		},
		{
			name: "project",
			key: func() (StageKey, error) {
				return BuildStageKey("project-b", "color", "v1", []byte("config-a"),
					[]ResolvedResource{lut}, map[string]string{"source": "sha256:source"})
			},
		},
		{
			name: "stage version",
			key: func() (StageKey, error) {
				return BuildStageKey("project-a", "color", "v2", []byte("config-a"),
					[]ResolvedResource{lut}, map[string]string{"source": "sha256:source"})
			},
		},
		{
			name: "config",
			key: func() (StageKey, error) {
				return BuildStageKey("project-a", "color", "v1", []byte("config-b"),
					[]ResolvedResource{lut}, map[string]string{"source": "sha256:source"})
			},
		},
		{
			name: "dependency key",
			key: func() (StageKey, error) {
				return BuildStageKey("project-a", "color", "v1", []byte("config-a"),
					[]ResolvedResource{lut}, map[string]string{"source": "sha256:changed"})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed, err := tc.key()
			if err != nil {
				t.Fatal(err)
			}
			if changed.Key == base.Key {
				t.Fatalf("changing %s did not change the cache key", tc.name)
			}
		})
	}
}

func TestBuildStageKeyFramesAdjacentFields(t *testing.T) {
	first, err := BuildStageKey("ab", "c", "v1", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildStageKey("a", "bc", "v1", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key == second.Key {
		t.Fatal("adjacent string fields were concatenated ambiguously")
	}
}

func TestBuildStageKeyRejectsConflictingDuplicateResources(t *testing.T) {
	ref := testRef("project-a", KindAudio, "bed.wav", "v1")
	_, err := BuildStageKey("project-a", "mix", "v1", nil,
		[]ResolvedResource{resolved(ref, "one"), resolved(ref, "two")}, nil)
	if !errors.Is(err, ErrInvalidCacheKeyInput) {
		t.Fatalf("error = %v, want ErrInvalidCacheKeyInput", err)
	}
}

func TestBuildStageKeyRejectsUnknownVersion(t *testing.T) {
	ref := testRef("project-a", KindFont, "title.otf", "")
	_, err := BuildStageKey("project-a", "text", "v1", nil,
		[]ResolvedResource{{Ref: ref, Fingerprint: FingerprintBytes([]byte("font"))}}, nil)
	if !errors.Is(err, ErrUnknownResourceVersion) {
		t.Fatalf("error = %v, want ErrUnknownResourceVersion", err)
	}
}

func TestBuildStageKeyRejectsUnknownFingerprint(t *testing.T) {
	ref := testRef("project-a", KindLUT, "show.cube", "v1")
	_, err := BuildStageKey("project-a", "color", "v1", nil,
		[]ResolvedResource{{Ref: ref, Fingerprint: "unknown"}}, nil)
	if !errors.Is(err, ErrInvalidCacheKeyInput) {
		t.Fatalf("error = %v, want ErrInvalidCacheKeyInput", err)
	}
}

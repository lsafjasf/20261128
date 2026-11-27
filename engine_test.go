package rendercache

import (
	"context"
	"errors"
	"testing"
)

type countingStore struct {
	Store
	gets int
}

func (s *countingStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	s.gets++
	return s.Store.Get(ctx, key)
}

type renderFixture struct {
	t        *testing.T
	project  string
	registry *Registry
	store    *countingStore
	engine   *Engine
	refs     map[string]ResourceRef
	calls    map[string]int
}

func newRenderFixture(t *testing.T, project string, sharedStore Store) *renderFixture {
	t.Helper()
	if sharedStore == nil {
		sharedStore = NewMemoryStore()
	}
	registry := NewRegistry()
	refs := map[string]ResourceRef{
		"lut":   testRef(project, KindLUT, "show.cube", "v1"),
		"font":  testRef(project, KindFont, "title.otf", "v1"),
		"audio": testRef(project, KindAudio, "bed.wav", "v1"),
	}
	for key, ref := range refs {
		if _, err := registry.Put(Resource{Ref: ref, Data: []byte(project + "/" + key + "-v1")}); err != nil {
			t.Fatal(err)
		}
	}
	store := &countingStore{Store: sharedStore}
	fixture := &renderFixture{
		t:        t,
		project:  project,
		registry: registry,
		store:    store,
		engine:   NewEngine(project, registry, store),
		refs:     refs,
		calls:    make(map[string]int),
	}
	for _, stage := range []Stage{
		fixture.audioStage("v1"),
		fixture.compositeStage(),
		fixture.colorStage("color-config", "v1"),
		fixture.textStage("v1"),
	} {
		if err := fixture.engine.AddStage(stage); err != nil {
			t.Fatal(err)
		}
	}
	return fixture
}

func (f *renderFixture) resourceStage(name, config, resourceName, resourceVersion string) Stage {
	ref := f.refs[resourceName]
	ref.Version = resourceVersion
	return Stage{
		Name:      name,
		Version:   "v1",
		Config:    []byte(config),
		Resources: []ResourceRef{ref},
		Run: func(_ context.Context, input StageInput) ([]byte, error) {
			f.calls[name]++
			snapshot := input.Resources[ref]
			if err := snapshot.VerifyFingerprint(); err != nil {
				return nil, err
			}
			return []byte(name + "(" + string(snapshot.Data) + ")"), nil
		},
	}
}

func (f *renderFixture) colorStage(config, resourceVersion string) Stage {
	return f.resourceStage("color", config, "lut", resourceVersion)
}

func (f *renderFixture) textStage(resourceVersion string) Stage {
	return f.resourceStage("text", "text-config", "font", resourceVersion)
}

func (f *renderFixture) audioStage(resourceVersion string) Stage {
	return f.resourceStage("audio", "audio-config", "audio", resourceVersion)
}

func (f *renderFixture) compositeStage() Stage {
	return Stage{
		Name:      "composite",
		Version:   "v1",
		Config:    []byte("composite-config"),
		DependsOn: []string{"audio", "color", "text"},
		Run: func(_ context.Context, input StageInput) ([]byte, error) {
			f.calls["composite"]++
			return []byte("composite|" +
				string(input.Dependencies["color"]) + "|" +
				string(input.Dependencies["text"]) + "|" +
				string(input.Dependencies["audio"])), nil
		},
	}
}

func (f *renderFixture) run() RunResult {
	f.t.Helper()
	result, err := f.engine.Run(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}

func assertReused(t *testing.T, result RunResult, want map[string]bool) {
	t.Helper()
	for name, reused := range want {
		if got := result.WasReused(name); got != reused {
			t.Errorf("stage %s reused = %v, want %v", name, got, reused)
		}
	}
}

func TestEnginePreciseInvalidation(t *testing.T) {
	f := newRenderFixture(t, "project-a", nil)
	allComputed := map[string]bool{
		"color": false, "text": false, "audio": false, "composite": false,
	}

	first := f.run()
	assertReused(t, first, allComputed)
	for name, calls := range f.calls {
		if calls != 1 {
			t.Fatalf("first run stage %s calls = %d, want 1", name, calls)
		}
	}

	second := f.run()
	allReused := map[string]bool{
		"color": true, "text": true, "audio": true, "composite": true,
	}
	assertReused(t, second, allReused)
	if string(second.Outputs["composite"]) != string(first.Outputs["composite"]) {
		t.Fatal("cache hit changed composite output")
	}

	if _, err := f.registry.Put(Resource{
		Ref: f.refs["lut"], Data: []byte("project-a/lut-v2"),
	}); err != nil {
		t.Fatal(err)
	}
	afterLUT := f.run()
	assertReused(t, afterLUT, map[string]bool{
		"color": false, "text": true, "audio": true, "composite": false,
	})
	if string(afterLUT.Outputs["composite"]) == string(first.Outputs["composite"]) {
		t.Fatal("LUT change did not reach composite output")
	}

	if _, err := f.registry.Put(Resource{
		Ref: f.refs["font"], Data: []byte("project-a/font-v2"),
	}); err != nil {
		t.Fatal(err)
	}
	afterFont := f.run()
	assertReused(t, afterFont, map[string]bool{
		"color": true, "text": false, "audio": true, "composite": false,
	})

	if _, err := f.registry.Put(Resource{
		Ref: f.refs["audio"], Data: []byte("project-a/audio-v2"),
	}); err != nil {
		t.Fatal(err)
	}
	afterAudio := f.run()
	assertReused(t, afterAudio, map[string]bool{
		"color": true, "text": true, "audio": false, "composite": false,
	})

	if err := f.engine.AddStage(f.colorStage("color-config-v2", "v1")); err != nil {
		t.Fatal(err)
	}
	afterConfig := f.run()
	assertReused(t, afterConfig, map[string]bool{
		"color": false, "text": true, "audio": true, "composite": false,
	})
}

func TestUnknownVersionPreventsAllCacheReads(t *testing.T) {
	f := newRenderFixture(t, "project-a", nil)
	f.run()
	if err := f.engine.AddStage(f.textStage("v2")); err != nil {
		t.Fatal(err)
	}
	f.store.gets = 0

	_, err := f.engine.Run(context.Background())
	if !errors.Is(err, ErrUnknownResourceVersion) {
		t.Fatalf("Run error = %v, want ErrUnknownResourceVersion", err)
	}
	if f.store.gets != 0 {
		t.Fatalf("unknown version allowed %d cache reads", f.store.gets)
	}

	_, err = f.engine.Plan()
	if !errors.Is(err, ErrUnknownResourceVersion) {
		t.Fatalf("Plan error = %v, want ErrUnknownResourceVersion", err)
	}
}

func TestProjectScopePreventsCrossProjectReuse(t *testing.T) {
	shared := NewMemoryStore()
	projectA := newRenderFixture(t, "project-a", shared)
	projectB := newRenderFixture(t, "project-b", shared)

	resultA := projectA.run()
	resultB := projectB.run()
	assertReused(t, resultB, map[string]bool{
		"color": false, "text": false, "audio": false, "composite": false,
	})
	for _, stage := range []string{"color", "text", "audio", "composite"} {
		if resultA.Keys[stage] == resultB.Keys[stage] {
			t.Errorf("stage %s reused a key across projects", stage)
		}
	}
}

func TestFileStoreReusesRenderAcrossEngineInstances(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := newRenderFixture(t, "project-a", store)
	first.run()

	second := newRenderFixture(t, "project-a", store)
	result := second.run()
	assertReused(t, result, map[string]bool{
		"color": true, "text": true, "audio": true, "composite": true,
	})
}

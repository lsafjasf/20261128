package rendercache

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrInvalidStage = errors.New("invalid stage")
	ErrStageGraph   = errors.New("invalid stage graph")
)

// StageInput contains the exact resource snapshots and direct upstream
// outputs for one stage execution.
type StageInput struct {
	Project      string
	Stage        string
	StageVersion string
	Config       []byte
	Resources    map[ResourceRef]ResourceSnapshot
	Dependencies map[string][]byte
}

// RunFunc renders one stage. It must be deterministic for a fixed StageInput.
type RunFunc func(context.Context, StageInput) ([]byte, error)

// Stage declares one cacheable render step. Version must change whenever the
// implementation or rendering semantics change without another keyed input
// changing. Config should contain a stable serialization of stage settings.
type Stage struct {
	Name      string
	Version   string
	Config    []byte
	Resources []ResourceRef
	DependsOn []string
	Run       RunFunc
}

type StageResult struct {
	Name     string
	Key      string
	Manifest Manifest
	Reused   bool
	Output   []byte
}

type RunResult struct {
	Project string
	Stages  []StageResult
	Outputs map[string][]byte
	Keys    map[string]string
}

func (r RunResult) WasReused(stage string) bool {
	for _, result := range r.Stages {
		if result.Name == stage {
			return result.Reused
		}
	}
	return false
}

// Engine evaluates one project's stage DAG against a registry and shared
// cache. Different projects use different keys, so they may safely share the
// same Store without reading each other's assets or outputs.
type Engine struct {
	Project  string
	registry *Registry
	store    Store

	mu     sync.RWMutex
	stages map[string]Stage
}

func NewEngine(project string, registry *Registry, store Store) *Engine {
	if registry == nil {
		registry = NewRegistry()
	}
	if store == nil {
		store = NewMemoryStore()
	}
	return &Engine{
		Project:  project,
		registry: registry,
		store:    store,
		stages:   make(map[string]Stage),
	}
}

// AddStage validates and adds or replaces a stage declaration.
func (e *Engine) AddStage(stage Stage) error {
	if strings.TrimSpace(stage.Name) == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidStage)
	}
	if strings.TrimSpace(stage.Version) == "" {
		return fmt.Errorf("%w: %s has an empty version", ErrInvalidStage, stage.Name)
	}
	if stage.Run == nil {
		return fmt.Errorf("%w: %s has no runner", ErrInvalidStage, stage.Name)
	}
	stage.Config = append([]byte(nil), stage.Config...)
	stage.Resources = append([]ResourceRef(nil), stage.Resources...)
	stage.DependsOn = append([]string(nil), stage.DependsOn...)

	e.mu.Lock()
	e.stages[stage.Name] = stage
	e.mu.Unlock()
	return nil
}

// Plan computes all stage keys without reading or writing cached output.
func (e *Engine) Plan() (map[string]StageKey, error) {
	plan, err := e.buildPlan()
	if err != nil {
		return nil, err
	}
	return plan.keys, nil
}

// Run reuses every valid stage and executes only missing or invalid stages.
// All resource versions are resolved before the first cache read, so one
// unknown version prevents any hit in the run.
func (e *Engine) Run(ctx context.Context) (RunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	plan, err := e.buildPlan()
	if err != nil {
		return RunResult{}, err
	}

	result := RunResult{
		Project: e.Project,
		Stages:  make([]StageResult, 0, len(plan.order)),
		Outputs: make(map[string][]byte, len(plan.order)),
		Keys:    make(map[string]string, len(plan.order)),
	}
	for _, name := range plan.order {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		stage := plan.stages[name]
		key := plan.keys[name]
		result.Keys[name] = key.Key

		data, found, err := e.store.Get(ctx, key.Key)
		if err != nil {
			return result, fmt.Errorf("stage %s cache lookup: %w", name, err)
		}
		if found {
			data = append([]byte(nil), data...)
			result.Stages = append(result.Stages, StageResult{
				Name: name, Key: key.Key, Manifest: key.Manifest, Reused: true, Output: data,
			})
			result.Outputs[name] = data
			continue
		}

		input := StageInput{
			Project:      e.Project,
			Stage:        name,
			StageVersion: stage.Version,
			Config:       append([]byte(nil), stage.Config...),
			Resources:    directResources(stage.Resources, plan.resources),
			Dependencies: dependencyOutputs(stage.DependsOn, result.Outputs),
		}
		data, err = stage.Run(ctx, input)
		if err != nil {
			return result, fmt.Errorf("run stage %s: %w", name, err)
		}
		data = append([]byte(nil), data...)
		if err := e.store.Put(ctx, key.Key, data); err != nil {
			return result, fmt.Errorf("stage %s cache write: %w", name, err)
		}
		result.Stages = append(result.Stages, StageResult{
			Name: name, Key: key.Key, Manifest: key.Manifest, Reused: false, Output: data,
		})
		result.Outputs[name] = data
	}
	return result, nil
}

type executionPlan struct {
	order     []string
	stages    map[string]Stage
	keys      map[string]StageKey
	resources map[ResourceRef]ResourceSnapshot
}

func (e *Engine) buildPlan() (executionPlan, error) {
	if strings.TrimSpace(e.Project) == "" {
		return executionPlan{}, fmt.Errorf("%w: project is empty", ErrInvalidCacheKeyInput)
	}
	stages := e.snapshotStages()
	order, err := orderStages(stages)
	if err != nil {
		return executionPlan{}, err
	}

	allRefs := make([]ResourceRef, 0)
	for _, stage := range stages {
		allRefs = append(allRefs, stage.Resources...)
	}
	resources, err := e.registry.SnapshotAll(allRefs)
	if err != nil {
		return executionPlan{}, err
	}

	plan := executionPlan{
		order:     order,
		stages:    stages,
		keys:      make(map[string]StageKey, len(order)),
		resources: resources,
	}
	for _, name := range order {
		stage := stages[name]
		direct := make([]ResolvedResource, 0, len(stage.Resources))
		seen := make(map[ResourceRef]struct{}, len(stage.Resources))
		for _, ref := range stage.Resources {
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			snapshot, ok := resources[ref]
			if !ok {
				return executionPlan{}, fmt.Errorf("stage %s: missing resource snapshot", name)
			}
			direct = append(direct, snapshot.ResolvedResource)
		}

		dependencies := make(map[string]string, len(stage.DependsOn))
		for _, dependency := range stage.DependsOn {
			dependencyKey, ok := plan.keys[dependency]
			if !ok {
				return executionPlan{}, fmt.Errorf("%w: stage %s dependency %s was not planned",
					ErrStageGraph, name, dependency)
			}
			dependencies[dependency] = dependencyKey.Key
		}
		key, err := BuildStageKey(e.Project, name, stage.Version, stage.Config, direct, dependencies)
		if err != nil {
			return executionPlan{}, fmt.Errorf("stage %s: %w", name, err)
		}
		plan.keys[name] = key
	}
	return plan, nil
}

func (e *Engine) snapshotStages() map[string]Stage {
	e.mu.RLock()
	defer e.mu.RUnlock()
	stages := make(map[string]Stage, len(e.stages))
	for name, stage := range e.stages {
		stage.Config = append([]byte(nil), stage.Config...)
		stage.Resources = append([]ResourceRef(nil), stage.Resources...)
		stage.DependsOn = append([]string(nil), stage.DependsOn...)
		stages[name] = stage
	}
	return stages
}

func orderStages(stages map[string]Stage) ([]string, error) {
	names := make([]string, 0, len(stages))
	for name := range stages {
		names = append(names, name)
	}
	sort.Strings(names)

	state := make(map[string]uint8, len(stages))
	order := make([]string, 0, len(stages))
	var visit func(string) error
	visit = func(name string) error {
		switch state[name] {
		case 1:
			return fmt.Errorf("%w: cycle includes stage %s", ErrStageGraph, name)
		case 2:
			return nil
		}
		state[name] = 1
		stage := stages[name]
		deps := append([]string(nil), stage.DependsOn...)
		sort.Strings(deps)
		last := ""
		for _, dependency := range deps {
			if dependency == last {
				continue
			}
			last = dependency
			if strings.TrimSpace(dependency) == "" {
				return fmt.Errorf("%w: stage %s has an empty dependency", ErrStageGraph, name)
			}
			if _, ok := stages[dependency]; !ok {
				return fmt.Errorf("%w: stage %s depends on unknown stage %s",
					ErrStageGraph, name, dependency)
			}
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = 2
		order = append(order, name)
		return nil
	}
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func directResources(refs []ResourceRef, snapshots map[ResourceRef]ResourceSnapshot) map[ResourceRef]ResourceSnapshot {
	direct := make(map[ResourceRef]ResourceSnapshot, len(refs))
	for _, ref := range refs {
		if snapshot, ok := snapshots[ref]; ok {
			snapshot.Data = append([]byte(nil), snapshot.Data...)
			direct[ref] = snapshot
		}
	}
	return direct
}

func dependencyOutputs(names []string, outputs map[string][]byte) map[string][]byte {
	direct := make(map[string][]byte, len(names))
	for _, name := range names {
		if data, ok := outputs[name]; ok {
			direct[name] = append([]byte(nil), data...)
		}
	}
	return direct
}

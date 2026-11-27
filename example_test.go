package rendercache_test

import (
	"context"
	"fmt"

	"rendercache"
)

func ExampleEngine() {
	registry := rendercache.NewRegistry()
	lut := rendercache.ResourceRef{
		Project: "film-a",
		Kind:    rendercache.KindLUT,
		Name:    "show.cube",
		Version: "v1",
	}
	_, _ = registry.Put(rendercache.Resource{Ref: lut, Data: []byte("lut-v1")})

	engine := rendercache.NewEngine("film-a", registry, rendercache.NewMemoryStore())
	_ = engine.AddStage(rendercache.Stage{
		Name:      "color",
		Version:   "1",
		Resources: []rendercache.ResourceRef{lut},
		Run: func(_ context.Context, input rendercache.StageInput) ([]byte, error) {
			return input.Resources[lut].Data, nil
		},
	})
	_ = engine.AddStage(rendercache.Stage{
		Name:      "composite",
		Version:   "1",
		DependsOn: []string{"color"},
		Run: func(_ context.Context, input rendercache.StageInput) ([]byte, error) {
			return []byte("frame:" + string(input.Dependencies["color"])), nil
		},
	})

	printRun := func() {
		result, err := engine.Run(context.Background())
		if err != nil {
			panic(err)
		}
		for _, stage := range result.Stages {
			fmt.Printf("%s=%v\n", stage.Name, stage.Reused)
		}
	}

	printRun()
	printRun()
	_, _ = registry.Put(rendercache.Resource{Ref: lut, Data: []byte("lut-v2")})
	printRun()

	// Output:
	// color=false
	// composite=false
	// color=true
	// composite=true
	// color=false
	// composite=false
}

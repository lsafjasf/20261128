package timeline

import "testing"

func TestBuildFreezesContentButAdvancesLabels(t *testing.T) {
	cfg := Config{
		Rate: "29.97DF", Width: 160, Height: 90, Frames: 12,
		Events: []Event{{Kind: Pause, Start: 4, Count: 3}, {Kind: Cut, Start: 8}},
	}
	plan, err := Build(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Frames[3].ContentFrame != 3 || plan.Frames[4].ContentFrame != 3 ||
		plan.Frames[5].ContentFrame != 3 || plan.Frames[6].ContentFrame != 3 {
		t.Fatalf("pause did not freeze content: %#v", plan.Frames[3:7])
	}
	if plan.Frames[7].ContentFrame != 4 {
		t.Fatalf("content did not resume: %#v", plan.Frames[7])
	}
	if plan.Frames[4].Stamp.Label == plan.Frames[5].Stamp.Label {
		t.Fatal("labels repeated during pause")
	}
	if plan.Frames[7].Clip == plan.Frames[8].Clip {
		t.Fatal("cut did not change clip")
	}
}

func TestRejectOverlapsAndHiddenCuts(t *testing.T) {
	cases := []Config{
		{Rate: "29.97DF", Width: 160, Height: 90, Frames: 20,
			Events: []Event{{Kind: Black, Start: 5, Count: 4}, {Kind: Pause, Start: 8, Count: 4}}},
		{Rate: "29.97DF", Width: 160, Height: 90, Frames: 20,
			Events: []Event{{Kind: Black, Start: 5, Count: 4}, {Kind: Cut, Start: 6}}},
		{Rate: "29.97DF", Width: 160, Height: 90, Frames: 20,
			Events: []Event{{Kind: Black, Start: 5, Count: 2}, {Kind: Cut, Start: 7}}},
		{Rate: "29.97DF", Width: 160, Height: 90, Frames: 20,
			Events: []Event{{Kind: Black, Start: 5, Count: 2}, {Kind: Pause, Start: 7, Count: 2}}},
		{Rate: "29.97DF", Width: 160, Height: 90, Frames: 20,
			Events: []Event{{Kind: Cut, Start: 5, Count: 1}}},
	}
	for _, cfg := range cases {
		if _, err := Build(cfg); err == nil {
			t.Fatalf("accepted invalid events: %#v", cfg.Events)
		}
	}
}

func TestAdjacentPauseEventsMerge(t *testing.T) {
	plan, err := Build(Config{
		Rate: "29.97DF", Width: 160, Height: 90, Frames: 20,
		Events: []Event{
			{Kind: Pause, Start: 4, Count: 2},
			{Kind: Pause, Start: 6, Count: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Events) != 1 || plan.Events[0].Start != 4 || plan.Events[0].Count != 5 {
		t.Fatalf("events = %#v", plan.Events)
	}
}

func TestDefaultConfigsAreValid(t *testing.T) {
	for _, frames := range []int{24, 30, 60, 90, 150, 300} {
		for _, rate := range []string{"23.976", "29.97DF"} {
			cfg := DefaultConfigSized(rate, 320, 180, frames)
			if _, err := Build(cfg); err != nil {
				t.Fatalf("%s/%d: %v (%#v)", rate, frames, err, cfg.Events)
			}
		}
	}
}

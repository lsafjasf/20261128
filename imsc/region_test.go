package imsc

import "testing"

func TestRegionTableMergesIdenticalDefinitions(t *testing.T) {
	tbl := NewRegionTable()
	r1, merged1 := tbl.Register(&Region{ID: "r1", Origin: "10px 20px", Extent: "100px 50px"})
	r2, merged2 := tbl.Register(&Region{ID: "r2", Origin: "10px 20px", Extent: "100px 50px"})

	if merged1 {
		t.Fatal("first registration should not merge")
	}
	if !merged2 {
		t.Fatal("structurally equal region must merge")
	}
	if r1 != r2 {
		t.Fatal("merged regions must be the same canonical instance")
	}
	got, ok := tbl.Get("r2")
	if !ok || got != r1 {
		t.Fatal("alias id must resolve to canonical region")
	}
	if tbl.Len() != 1 {
		t.Fatalf("expected 1 canonical region, got %d", tbl.Len())
	}
}

func TestRegionTableKeepsDifferentDefinitionsSeparate(t *testing.T) {
	tbl := NewRegionTable()
	tbl.Register(&Region{ID: "r1", Origin: "10px 20px"})
	tbl.Register(&Region{ID: "r2", Origin: "99px 88px"})
	if tbl.Len() != 2 {
		t.Fatalf("distinct regions must stay separate, got %d", tbl.Len())
	}
}

func TestRegionFallbackChain(t *testing.T) {
	tbl := NewRegionTable()
	tbl.Register(&Region{ID: "safe", Origin: "0px 0px"})
	tbl.Register(&Region{ID: "preferred", Origin: "5px 5px", FallbackID: "safe"})

	chain := tbl.Chain("preferred")
	if len(chain) != 2 || chain[0] != "preferred" || chain[1] != "safe" {
		t.Fatalf("unexpected chain: %v", chain)
	}

	// An unknown region resolves to the default region terminator.
	rg, err := tbl.Resolve("does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rg.ID != DefaultRegionID {
		t.Fatalf("expected default region, got %q", rg.ID)
	}
}

func TestRegionFallbackCycle(t *testing.T) {
	tbl := NewRegionTable()
	tbl.Register(&Region{ID: "a", FallbackID: "b"})
	tbl.Register(&Region{ID: "b", FallbackID: "a"})
	if _, err := tbl.Resolve("a"); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestRegionReferenceCounting(t *testing.T) {
	tbl := NewRegionTable()
	tbl.Register(&Region{ID: "r1", Origin: "1px 1px"})
	for i := 0; i < 3; i++ {
		if _, err := tbl.AddRef("r1"); err != nil {
			t.Fatal(err)
		}
	}
	rg, _ := tbl.Get("r1")
	if rg.RefCount != 3 {
		t.Fatalf("expected 3 refs, got %d", rg.RefCount)
	}
}

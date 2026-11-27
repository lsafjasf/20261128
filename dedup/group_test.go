package dedup

import (
	"testing"

	"mediadedup/media"
)

func TestBuildGroups(t *testing.T) {
	pool := Build(poolFixtures(), DefaultThresholds())
	if len(pool.Groups) != 2 {
		t.Fatalf("groups=%d want 2", len(pool.Groups))
	}

	g1 := pool.Groups[0]
	wantMembers := map[string]bool{"A1": true, "A2": true, "A3": true}
	if len(g1.Members) != len(wantMembers) {
		t.Fatalf("G1 members=%d want 3", len(g1.Members))
	}
	for _, m := range g1.Members {
		if !wantMembers[m.ID] {
			t.Fatalf("unexpected G1 member %s", m.ID)
		}
	}

	g2 := pool.Groups[1]
	if len(g2.Members) != 2 || !g2.HasMember("B1") || !g2.HasMember("B2") {
		t.Fatalf("G2 members=%v want B1,B2", g2.Members)
	}

	// Singletons must not appear in any group.
	for _, id := range []string{"C1", "D1"} {
		if _, ok := pool.GroupOf(id); ok {
			t.Fatalf("singleton %s unexpectedly grouped", id)
		}
	}
}

func TestGroupPairRelations(t *testing.T) {
	pool := Build(poolFixtures(), DefaultThresholds())
	g := pool.Groups[0]

	r12, ok := g.MemberRelation("A1", "A2")
	if !ok || r12.Relation != RelationTranscode {
		t.Fatalf("A1-A2 relation=%v ok=%v want transcode (rotation+fps+codec changed)", r12.Relation, ok)
	}
	r13, ok := g.MemberRelation("A1", "A3")
	if !ok || r13.Relation != RelationTranscode {
		t.Fatalf("A1-A3 relation=%v ok=%v want transcode", r13.Relation, ok)
	}
}

func TestDifferentShotsNeverMerge(t *testing.T) {
	pool := Build(poolFixtures(), DefaultThresholds())
	for _, g := range pool.Groups {
		for key, r := range g.Relations {
			if r.Relation == RelationDifferent {
				t.Fatalf("group %s contains different pair %s", g.ID, key)
			}
		}
	}
}

func TestRecommendMasterAndSetMaster(t *testing.T) {
	pool := Build(poolFixtures(), DefaultThresholds())
	g := pool.Groups[0]
	// A1: 1080p30 prores 45M unrotated; A2: rotated h264 8M; A3: 1080p60 12M.
	if g.MasterID != "A3" {
		t.Fatalf("recommended master=%s want A3 (highest frame rate)", g.MasterID)
	}
	if err := g.SetMaster("A1"); err != nil {
		t.Fatalf("SetMaster(A1): %v", err)
	}
	if g.MasterID != "A1" {
		t.Fatalf("master=%s want A1", g.MasterID)
	}
	if err := g.SetMaster("Z9"); err == nil {
		t.Fatal("SetMaster with foreign asset must fail")
	}
}

func TestCandidatePairWindowing(t *testing.T) {
	assets := []media.Asset{
		{ID: "s1", Duration: 1.0},
		{ID: "s2", Duration: 1.1},
		{ID: "s3", Duration: 1.2},
		{ID: "s4", Duration: 5.0},
	}
	pairs := candidatePairs(assets, 0.25)
	seen := map[string]bool{}
	for _, p := range pairs {
		seen[p[0]+"|"+p[1]] = true
	}
	expected := []string{"s1|s2", "s1|s3", "s2|s3"}
	for _, k := range expected {
		if !seen[k] {
			t.Fatalf("missing expected pair %s", k)
		}
	}
	if seen["s3|s4"] || seen["s1|s4"] || seen["s2|s4"] {
		t.Fatalf("far-duration pair leaked: %v", seen)
	}
	if len(pairs) != len(expected) {
		t.Fatalf("pairs=%d want %d (%v)", len(pairs), len(expected), pairs)
	}
}

func TestUnionFindTransitivity(t *testing.T) {
	uf := newUnionFind([]string{"a", "b", "c"})
	uf.union("a", "b")
	uf.union("b", "c")
	if uf.find("a") != uf.find("c") {
		t.Fatal("transitive union failed")
	}
}

func TestEmptyPool(t *testing.T) {
	pool := Build(nil, DefaultThresholds())
	if len(pool.Groups) != 0 {
		t.Fatalf("empty pool produced groups: %d", len(pool.Groups))
	}
}

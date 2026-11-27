package medialib

import (
	"errors"
	"testing"
)

func TestSameRangeDoesNotMerge(t *testing.T) {
	lib := NewLibrary()
	masterID, err := lib.AddMaster("interview.mov", "/media/interview.mov", 600)
	if err != nil {
		t.Fatalf("AddMaster: %v", err)
	}

	rng := TimeRange{Start: 100, End: 200}
	clipA, err := lib.CutSubclip(masterID, "quote-A", rng, map[string]string{"color": "red"})
	if err != nil {
		t.Fatalf("CutSubclip A: %v", err)
	}
	clipB, err := lib.CutSubclip(masterID, "quote-B", rng, map[string]string{"color": "blue"})
	if err != nil {
		t.Fatalf("CutSubclip B: %v", err)
	}

	if clipA == clipB || clipA.String() == clipB.String() {
		t.Fatalf("identical source range produced merged identities: %s", clipA)
	}

	got := lib.SubclipsOf(masterID)
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct subclips for the same range, got %d: %v", len(got), got)
	}

	a, _ := lib.Subclip(clipA)
	b, _ := lib.Subclip(clipB)
	if a.Range != b.Range || a.Master != b.Master {
		t.Fatalf("source attributes differ unexpectedly: %+v vs %+v", a.Range, b.Range)
	}
	if a.Name == b.Name || a.Markers["color"] == b.Markers["color"] {
		t.Fatalf("names/markers must stay independent per identity")
	}

	if err := lib.VerifyIntegrity(); err != nil {
		t.Fatalf("integrity: %v", err)
	}
}

func TestIDsAreOpaqueAndStableAcrossRename(t *testing.T) {
	lib := NewLibrary()
	masterID, _ := lib.AddMaster("old-name.mov", "/p/x.mov", 100)
	clipID, _ := lib.CutSubclip(masterID, "clip", TimeRange{0, 50}, nil)
	instID, err := lib.AddInstance(clipID, "main", 0, 50)
	if err != nil {
		t.Fatalf("AddInstance: %v", err)
	}

	if err := lib.Rename(masterID, "new-name.mov"); err != nil {
		t.Fatalf("rename master: %v", err)
	}
	if err := lib.Rename(clipID, "clip-renamed"); err != nil {
		t.Fatalf("rename subclip: %v", err)
	}

	m, err := lib.Master(masterID)
	if err != nil {
		t.Fatalf("master lookup by original ID after rename: %v", err)
	}
	if m.Name != "new-name.mov" {
		t.Fatalf("rename did not apply: %q", m.Name)
	}
	// The instance still resolves through the unchanged subclip ID.
	if refs := lib.InstancesOf(clipID); len(refs) != 1 || refs[0] != instID {
		t.Fatalf("reference broken after rename: %v", refs)
	}
	if err := lib.VerifyIntegrity(); err != nil {
		t.Fatalf("integrity after rename: %v", err)
	}
}

func TestDanglingRefsRejected(t *testing.T) {
	lib := NewLibrary()
	bogus := ID{Kind: KindMaster, Hex: "deadbeef"}
	if _, err := lib.CutSubclip(bogus, "x", TimeRange{0, 10}, nil); !errors.Is(err, ErrDanglingRef) {
		t.Fatalf("expected ErrDanglingRef, got %v", err)
	}

	masterID, _ := lib.AddMaster("m.mov", "/p", 100)
	clipID, _ := lib.CutSubclip(masterID, "c", TimeRange{0, 10}, nil)
	bogusClip := ID{Kind: KindSubclip, Hex: "deadbeef"}
	if _, err := lib.AddInstance(bogusClip, "tl", 0, 5); !errors.Is(err, ErrDanglingRef) {
		t.Fatalf("expected ErrDanglingRef for instance, got %v", err)
	}
	if err := lib.Retrim(clipID, TimeRange{50, 200}); !errors.Is(err, ErrRangeOutOfBounds) {
		t.Fatalf("expected ErrRangeOutOfBounds, got %v", err)
	}
	if err := lib.Retrim(clipID, TimeRange{80, 40}); !errors.Is(err, ErrBadRange) {
		t.Fatalf("expected ErrBadRange, got %v", err)
	}
}

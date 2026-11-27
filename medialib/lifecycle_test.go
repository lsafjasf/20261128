package medialib

import (
	"errors"
	"testing"
)

func TestDuplicateSubclipIsIndependentIdentity(t *testing.T) {
	lib := NewLibrary()
	masterID, _ := lib.AddMaster("m.mov", "/p", 500)
	src, _ := lib.CutSubclip(masterID, "orig", TimeRange{10, 90}, map[string]string{"tag": "v1"})

	copyID, err := lib.DuplicateSubclip(src, "orig-copy")
	if err != nil {
		t.Fatalf("DuplicateSubclip: %v", err)
	}
	if copyID == src {
		t.Fatal("duplicate must receive a new stable ID")
	}

	// Mutating the copy's markers/name must not leak into the original.
	if err := lib.SetMarker(copyID, "tag", "v2"); err != nil {
		t.Fatalf("SetMarker: %v", err)
	}
	if err := lib.Retrim(copyID, TimeRange{20, 80}); err != nil {
		t.Fatalf("Retrim copy: %v", err)
	}

	orig, _ := lib.Subclip(src)
	dup, _ := lib.Subclip(copyID)
	if orig.Markers["tag"] != "v1" {
		t.Fatalf("original marker mutated through duplicate: %q", orig.Markers["tag"])
	}
	if dup.Range == orig.Range {
		t.Fatal("retrim on the duplicate unexpectedly changed the original range")
	}
	if len(lib.SubclipsOf(masterID)) != 2 {
		t.Fatal("duplicate with overlapping range must not merge into the source")
	}
}

func TestRetrimPreservesInstanceReferences(t *testing.T) {
	lib := NewLibrary()
	masterID, _ := lib.AddMaster("m.mov", "/p", 500)
	clipID, _ := lib.CutSubclip(masterID, "clip", TimeRange{0, 100}, nil)

	var instIDs []ID
	for i := 0; i < 3; i++ {
		id, err := lib.AddInstance(clipID, "main", int64(i*100), 100)
		if err != nil {
			t.Fatalf("AddInstance %d: %v", i, err)
		}
		instIDs = append(instIDs, id)
	}

	if err := lib.Retrim(clipID, TimeRange{40, 120}); err != nil {
		t.Fatalf("Retrim: %v", err)
	}
	got := lib.InstancesOf(clipID)
	if len(got) != len(instIDs) {
		t.Fatalf("retrim altered instance set: got %d want %d", len(got), len(instIDs))
	}
	clip, _ := lib.Subclip(clipID)
	if clip.Range != (TimeRange{Start: 40, End: 120}) {
		t.Fatalf("range not updated: %+v", clip.Range)
	}
	for _, id := range instIDs {
		ins, err := lib.Instance(id)
		if err != nil {
			t.Fatalf("instance %s vanished after retrim: %v", id, err)
		}
		if ins.Subclip != clipID {
			t.Fatalf("instance relinked to %s after retrim", ins.Subclip)
		}
	}
	if err := lib.VerifyIntegrity(); err != nil {
		t.Fatalf("integrity: %v", err)
	}
}

func TestDeleteGuardsAndCascade(t *testing.T) {
	lib := NewLibrary()
	masterID, _ := lib.AddMaster("m.mov", "/p", 500)
	clipA, _ := lib.CutSubclip(masterID, "A", TimeRange{0, 50}, nil)
	clipB, _ := lib.CutSubclip(masterID, "B", TimeRange{0, 50}, nil) // same range, distinct ID
	instA, _ := lib.AddInstance(clipA, "tl", 0, 50)

	if err := lib.DeleteSubclip(clipA, false); !errors.Is(err, ErrInUse) {
		t.Fatalf("expected ErrInUse deleting referenced subclip, got %v", err)
	}
	if err := lib.DeleteMaster(masterID, false); !errors.Is(err, ErrInUse) {
		t.Fatalf("expected ErrInUse deleting master with subclips, got %v", err)
	}

	if err := lib.DeleteInstance(instA); err != nil {
		t.Fatalf("DeleteInstance: %v", err)
	}
	if err := lib.DeleteSubclip(clipA, false); err != nil {
		t.Fatalf("delete subclip after removing instances: %v", err)
	}
	if _, err := lib.Subclip(clipA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	// clipB shares the same source range but is an independent identity and survives.
	if _, err := lib.Subclip(clipB); err != nil {
		t.Fatalf("sibling subclip wrongly removed: %v", err)
	}

	if err := lib.DeleteMaster(masterID, true); err != nil {
		t.Fatalf("cascade delete master: %v", err)
	}
	if _, err := lib.Master(masterID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("master still present: %v", err)
	}
	if _, err := lib.Subclip(clipB); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cascade did not remove dependent subclip: %v", err)
	}
	if err := lib.VerifyIntegrity(); err != nil {
		t.Fatalf("integrity after cascade: %v", err)
	}
}

func TestEndToEndReferenceLifecycle(t *testing.T) {
	lib := NewLibrary()

	masterID, err := lib.AddMaster("take-01.mov", "/media/take-01.mov", 900)
	if err != nil {
		t.Fatal(err)
	}
	// Two subclips from the SAME master over the SAME range, different names/markers.
	rng := TimeRange{Start: 120, End: 300}
	clip1, _ := lib.CutSubclip(masterID, "opening", rng, map[string]string{"mood": "calm"})
	clip2, _ := lib.CutSubclip(masterID, "opening-alt", rng, map[string]string{"mood": "tense"})

	inst1, _ := lib.AddInstance(clip1, "sequence-1", 0, 180)
	inst2, _ := lib.AddInstance(clip1, "sequence-1", 180, 180)
	inst3, _ := lib.AddInstance(clip2, "sequence-2", 0, 180)

	// 1. Rename everything: IDs and references remain valid.
	_ = lib.Rename(masterID, "take-01-final.mov")
	_ = lib.Rename(clip1, "opening-v2")

	// 2. Duplicate clip2, then rename/retrim the copy; original untouched.
	clip2copy, _ := lib.DuplicateSubclip(clip2, "opening-alt-copy")
	_ = lib.Retrim(clip2copy, TimeRange{150, 280})

	// 3. Retrim clip1; inst1 and inst2 keep referencing the same subclip ID.
	if err := lib.Retrim(clip1, TimeRange{120, 260}); err != nil {
		t.Fatal(err)
	}

	// 4. Delete one instance, then non-cascade-delete the subclip it shared.
	if err := lib.DeleteInstance(inst2); err != nil {
		t.Fatal(err)
	}
	if err := lib.DeleteSubclip(clip1, false); !errors.Is(err, ErrInUse) {
		t.Fatalf("clip1 still used by inst1, got %v", err)
	}
	_ = lib.DeleteInstance(inst1)
	if err := lib.DeleteSubclip(clip1, false); err != nil {
		t.Fatalf("delete clip1 once unreferenced: %v", err)
	}

	// clip2 and its instance are unaffected by clip1's removal.
	if refs := lib.InstancesOf(clip2); len(refs) != 1 || refs[0] != inst3 {
		t.Fatalf("clip2 references disturbed: %v", refs)
	}
	// The duplicate is independent and still resolvable.
	if _, err := lib.Subclip(clip2copy); err != nil {
		t.Fatalf("duplicate lost: %v", err)
	}
	if err := lib.VerifyIntegrity(); err != nil {
		t.Fatalf("final integrity: %v", err)
	}
}

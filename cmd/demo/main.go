package main

import (
	"errors"
	"fmt"

	"github.com/example/medialib-demo/medialib"
)

func main() {
	lib := medialib.NewLibrary()

	masterID, _ := lib.AddMaster("take-01.mov", "/media/take-01.mov", 900)
	rng := medialib.TimeRange{Start: 120, End: 300}

	// Same master, same source range, different names/markers -> distinct IDs.
	clipA, _ := lib.CutSubclip(masterID, "opening", rng, map[string]string{"mood": "calm"})
	clipB, _ := lib.CutSubclip(masterID, "opening-alt", rng, map[string]string{"mood": "tense"})
	fmt.Printf("master : %s\n", masterID)
	fmt.Printf("clip A : %s  (same range as B)\n", clipA)
	fmt.Printf("clip B : %s  (same range as A)\n", clipB)
	fmt.Printf("identities kept separate: %v\n\n", clipA != clipB)

	instA1, _ := lib.AddInstance(clipA, "seq-1", 0, 180)
	instA2, _ := lib.AddInstance(clipA, "seq-1", 180, 180)
	instB, _ := lib.AddInstance(clipB, "seq-2", 0, 180)

	// Rename: stable IDs, references unaffected.
	_ = lib.Rename(masterID, "take-01-final.mov")
	_ = lib.Rename(clipA, "opening-v2")

	// Duplicate B: independent identity and mutable state.
	clipBCopy, _ := lib.DuplicateSubclip(clipB, "opening-alt-copy")
	_ = lib.SetMarker(clipBCopy, "mood", "neutral")
	_ = lib.Retrim(clipBCopy, medialib.TimeRange{Start: 150, End: 280})

	// Retrim A: both instances still point at clipA's unchanged ID.
	_ = lib.Retrim(clipA, medialib.TimeRange{Start: 120, End: 260})
	fmt.Printf("after retrim, instances of A: %d (IDs unchanged)\n\n", len(lib.InstancesOf(clipA)))

	// Guarded delete: referenced subclip cannot be removed.
	if err := lib.DeleteSubclip(clipA, false); errors.Is(err, medialib.ErrInUse) {
		fmt.Printf("delete blocked while referenced: %v\n", err)
	}
	_ = lib.DeleteInstance(instA1)
	_ = lib.DeleteInstance(instA2)
	_ = lib.DeleteSubclip(clipA, false)

	// B (identical source range to the deleted A) and its instance survive.
	if refs := lib.InstancesOf(clipB); len(refs) == 1 && refs[0] == instB {
		fmt.Println("clip B reference still valid after clip A deletion")
	}

	// Cascade removes master -> remaining dependent subclips/instances.
	if err := lib.DeleteMaster(masterID, true); err != nil {
		fmt.Println("cascade error:", err)
	}

	if err := lib.VerifyIntegrity(); err != nil {
		fmt.Println("INTEGRITY FAILURE:", err)
		return
	}
	fmt.Println("integrity OK: no dangling references, no range-based merging")
}

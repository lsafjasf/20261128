package plan

import (
	"path/filepath"
	"testing"

	"clipmerge/internal/conflict"
	"clipmerge/internal/manifest"
)

func TestBuildAndApply(t *testing.T) {
	m := &manifest.Manifest{
		Media: []manifest.Media{
			{UID: "U1", Path: "day1/c1.mov", HashAlgo: "sha256", Hash: "h1", ShootDay: "day1"},
			{UID: "U2", Path: "day2/c1.mov", HashAlgo: "sha256", Hash: "h2", ShootDay: "day2"},
			{UID: "U3", Path: "day1/c2.mov", HashAlgo: "sha256", Hash: "h3", ShootDay: "day1"},
		},
		Clips: []manifest.Clip{
			{ID: "C1", ShootDay: "day1", Bin: "B", ClipNo: "001", LockedUID: "U1", LockedHash: "h1"},
			{ID: "C2", ShootDay: "day2", Bin: "B", ClipNo: "001", LockedUID: "U1", LockedHash: "h1", ExpectedUID: "U2"},
			{ID: "C3", ShootDay: "day1", Bin: "B", ClipNo: "002", LockedUID: "U3", LockedHash: "h3"},
			{ID: "C4", ShootDay: "day1", Bin: "B", ClipNo: "002", LockedUID: "U3", LockedHash: "h3"},
		},
	}

	rep := conflict.Analyze(m, "")
	p := Build(m, rep)

	if len(p.Rollback) != 1 || p.Rollback[0].ClipID != "C2" || p.Rollback[0].ToUID != "U2" {
		t.Fatalf("unexpected rollbacks: %+v", p.Rollback)
	}
	newNos := map[string]string{}
	for _, r := range p.Renames {
		newNos[r.ClipID] = r.NewClipNo
	}
	if newNos["C1"] != "001_day1" || newNos["C2"] != "001_day2" {
		t.Fatalf("cross-day rename wrong: %v", newNos)
	}
	if newNos["C3"] == newNos["C4"] {
		t.Fatalf("same-day duplicates must get distinct new numbers: %v", newNos)
	}

	out := filepath.Join(t.TempDir(), "corrected.json")
	rolled, renamed, err := Apply(m, p, out)
	if err != nil {
		t.Fatal(err)
	}
	if rolled != 1 || renamed != 3 {
		t.Fatalf("apply counts wrong: rolled=%d renamed=%d", rolled, renamed)
	}

	fixed, err := manifest.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]manifest.Clip{}
	for _, c := range fixed.Clips {
		byID[c.ID] = c
	}
	if byID["C2"].LockedUID != "U2" || byID["C2"].LockedHash != "h2" {
		t.Fatalf("rollback not applied: %+v", byID["C2"])
	}
	if byID["C1"].ClipNo != "001_day1" {
		t.Fatalf("rename not applied: %+v", byID["C1"])
	}

	// Second analysis of the corrected manifest: namespace/lock conflicts gone.
	rep2 := conflict.Analyze(fixed, "")
	for _, g := range rep2.Groups {
		switch g.Kind {
		case conflict.KindNamespaceCollision, conflict.KindFullKeyDuplicate,
			conflict.KindWrongLock, conflict.KindUIDReuse:
			t.Fatalf("conflict %s survived remediation: %+v", g.Kind, g)
		}
	}

	// Original manifest must remain untouched.
	if m.Clips[1].LockedUID != "U1" || m.Clips[0].ClipNo != "001" {
		t.Fatal("Apply mutated the input manifest")
	}
}

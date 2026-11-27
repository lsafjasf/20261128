package conflict

import (
	"testing"

	"clipmerge/internal/manifest"
)

func sampleManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Media: []manifest.Media{
			{UID: "U1", Path: "a/c1.mov", HashAlgo: "sha256", Hash: "h1", ShootDay: "day1"},
			{UID: "U2", Path: "b/c1.mov", HashAlgo: "sha256", Hash: "h2", ShootDay: "day2"},
			{UID: "U3", Path: "a/c2.mov", HashAlgo: "sha256", Hash: "h2", ShootDay: "day1"},
		},
		Clips: []manifest.Clip{
			{ID: "C1", ShootDay: "day1", Bin: "B", ClipNo: "001", LockedUID: "U1", LockedHash: "h1"},
			{ID: "C2", ShootDay: "day2", Bin: "B", ClipNo: "001", LockedUID: "U1", LockedHash: "h1", ExpectedUID: "U2"},
			{ID: "C3", ShootDay: "day1", Bin: "B", ClipNo: "002", LockedUID: "U3", LockedHash: "h2"},
			{ID: "C4", ShootDay: "day1", Bin: "B", ClipNo: "003", LockedUID: "GONE", LockedHash: "hx"},
		},
	}
}

func kinds(rep *Report) map[string]int {
	out := map[string]int{}
	for _, g := range rep.Groups {
		out[g.Kind]++
	}
	return out
}

func TestAnalyzeRegistryOnly(t *testing.T) {
	rep := Analyze(sampleManifest(), "")
	got := kinds(rep)

	want := map[string]int{
		KindNamespaceCollision: 1,
		KindUIDReuse:           1,
		KindHashAlias:          1,
		KindLockDangling:       1,
		KindWrongLock:          1,
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("kind %s: got %d want %d (all=%v)", k, got[k], v, got)
		}
	}

	var wl WrongLock
	for _, g := range rep.Groups {
		if g.Kind == KindWrongLock {
			if len(g.WrongLocks) != 1 {
				t.Fatalf("expected 1 wrong lock, got %d", len(g.WrongLocks))
			}
			wl = g.WrongLocks[0]
		}
	}
	if wl.ClipID != "C2" || wl.CorrectUID != "U2" || wl.ResolvedBy != "expectedUID" {
		t.Fatalf("unexpected wrong lock resolution: %+v", wl)
	}
}

func TestAnalyzeBlockedWhenNoSameDayTarget(t *testing.T) {
	m := sampleManifest()
	// C2 cannot resolve: expected UID removed, locked hash has no day2 media.
	m.Clips[1].ExpectedUID = ""
	m.Clips[1].LockedHash = "h1"

	rep := Analyze(m, "")
	got := kinds(rep)
	if got[KindRollbackBlocked] != 1 {
		t.Fatalf("expected 1 blocked rollback, got %v", got)
	}
	if got[KindWrongLock] != 0 {
		t.Fatalf("blocked locks must not appear as resolvable wrong locks, got %v", got)
	}
}

func TestAnalyzeFullKeyDuplicateSameDay(t *testing.T) {
	m := &manifest.Manifest{
		Media: []manifest.Media{
			{UID: "U1", Path: "a", HashAlgo: "sha256", Hash: "h1", ShootDay: "day1"},
			{UID: "U2", Path: "b", HashAlgo: "sha256", Hash: "h2", ShootDay: "day1"},
		},
		Clips: []manifest.Clip{
			{ID: "C1", ShootDay: "day1", Bin: "B", ClipNo: "001", LockedUID: "U1", LockedHash: "h1"},
			{ID: "C2", ShootDay: "day1", Bin: "B", ClipNo: "001", LockedUID: "U2", LockedHash: "h2"},
		},
	}
	rep := Analyze(m, "")
	got := kinds(rep)
	if got[KindFullKeyDuplicate] != 1 {
		t.Fatalf("expected full-key duplicate, got %v", got)
	}
	if got[KindNamespaceCollision] != 0 {
		t.Fatalf("same-day reuse must not be classified as cross-day collision: %v", got)
	}
}

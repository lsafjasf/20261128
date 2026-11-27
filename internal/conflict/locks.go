package conflict

import (
	"fmt"
	"slices"

	"clipmerge/internal/manifest"
)

func (d *detector) scanLocks() {
	wrongByNS := map[string][]WrongLock{}
	blocked := []WrongLock{}

	for _, c := range d.orderedClips() {
		locked, ok := d.idx.MediaByUID[c.LockedUID]
		if !ok {
			d.groups = append(d.groups, Group{
				Kind:      KindLockDangling,
				Severity:  SeverityCritical,
				Reason:    "clip locks media UID that does not exist in the merged registry",
				ClipIDs:   []string{c.ID},
				MediaUIDs: []string{c.LockedUID},
			})
			continue
		}

		wl, wrong := d.evaluateLock(c, locked)
		if !wrong {
			continue
		}
		if wl.ResolvedBy == "blocked" {
			blocked = append(blocked, wl)
			continue
		}
		ns := c.Bin + "/" + c.ClipNo
		wrongByNS[ns] = append(wrongByNS[ns], wl)
	}

	for ns, locks := range wrongByNS {
		clipIDs := make([]string, 0, len(locks))
		uids := map[string]bool{}
		for _, wl := range locks {
			clipIDs = append(clipIDs, wl.ClipID)
			uids[wl.LockedUID] = true
			if wl.CorrectUID != "" {
				uids[wl.CorrectUID] = true
			}
		}
		d.groups = append(d.groups, Group{
			Kind:       KindWrongLock,
			Severity:   SeverityCritical,
			Reason:     "clip is locked to media belonging to another shoot day (cross-wired); explicit rollback required",
			Namespace:  ns,
			ClipIDs:    sorted(clipIDs),
			MediaUIDs:  sortedKeys(uids),
			WrongLocks: sortedLocks(locks),
		})
	}

	for _, wl := range blocked {
		d.groups = append(d.groups, Group{
			Kind:       KindRollbackBlocked,
			Severity:   SeverityCritical,
			Reason:     "wrong lock cannot be rolled back automatically: no unambiguous same-day target",
			ClipIDs:    []string{wl.ClipID},
			MediaUIDs:  []string{wl.LockedUID},
			WrongLocks: []WrongLock{wl},
		})
	}
}

func (d *detector) evaluateLock(c *manifest.Clip, locked *manifest.Media) (WrongLock, bool) {
	wrong := false
	reason := ""

	if c.LockedHash != "" && locked.Hash != "" && c.LockedHash != locked.Hash {
		wrong = true
		reason = fmt.Sprintf("clip locked hash %s differs from media %s declared hash %s", c.LockedHash, locked.UID, locked.Hash)
	}
	if actual, ok := d.actual[locked.UID]; ok {
		declared := locked.Hash
		if declared == "" {
			declared = c.LockedHash
		}
		if actual != declared {
			wrong = true
			reason = fmt.Sprintf("on-disk content of %s hashes to %s, expected %s (content swapped)", locked.UID, actual, declared)
		}
	}
	if locked.ShootDay != "" && locked.ShootDay != c.ShootDay {
		wrong = true
		reason = fmt.Sprintf("clip shoot day %s but locked media %s belongs to shoot day %s", c.ShootDay, locked.UID, locked.ShootDay)
	}
	if c.ExpectedUID != "" && c.ExpectedUID != c.LockedUID {
		wrong = true
		reason = fmt.Sprintf("clip expected media %s but is locked to %s", c.ExpectedUID, c.LockedUID)
	}
	if !wrong {
		return WrongLock{}, false
	}

	wl := WrongLock{
		ClipID:     c.ID,
		ShootDay:   c.ShootDay,
		LockedUID:  c.LockedUID,
		LockedHash: c.LockedHash,
		Reason:     reason,
	}
	if md := d.idx.MediaByUID[c.ExpectedUID]; md != nil && md.ShootDay == c.ShootDay && md.UID != c.LockedUID {
		wl.CorrectUID = md.UID
		wl.CorrectHash = md.Hash
		wl.ResolvedBy = "expectedUID"
		return wl, true
	}

	if c.LockedHash != "" {
		var candidates []*manifest.Media
		for _, md := range d.idx.MediaByHash[c.LockedHash] {
			if md.ShootDay == c.ShootDay && md.UID != c.LockedUID {
				candidates = append(candidates, md)
			}
		}
		if len(candidates) == 1 {
			wl.CorrectUID = candidates[0].UID
			wl.CorrectHash = candidates[0].Hash
			wl.ResolvedBy = "hashLookup"
			return wl, true
		}
	}
	wl.ResolvedBy = "blocked"
	return wl, true
}

func (d *detector) orderedClips() []*manifest.Clip {
	out := make([]*manifest.Clip, 0, len(d.idx.ClipsByID))
	for _, c := range d.idx.ClipsByID {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b *manifest.Clip) int {
		switch {
		case a.ShootDay != b.ShootDay:
			return cmpString(a.ShootDay, b.ShootDay)
		case a.Bin != b.Bin:
			return cmpString(a.Bin, b.Bin)
		case a.ClipNo != b.ClipNo:
			return cmpString(a.ClipNo, b.ClipNo)
		default:
			return cmpString(a.ID, b.ID)
		}
	})
	return out
}

func (d *detector) assignIDs() {
	slices.SortFunc(d.groups, func(a, b Group) int {
		if a.Kind != b.Kind {
			return cmpString(a.Kind, b.Kind)
		}
		return cmpString(a.Namespace, b.Namespace)
	})
	for i := range d.groups {
		d.groups[i].ID = fmt.Sprintf("CG-%03d", i+1)
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func sortedLocks(in []WrongLock) []WrongLock {
	out := append([]WrongLock(nil), in...)
	slices.SortFunc(out, func(a, b WrongLock) int { return cmpString(a.ClipID, b.ClipID) })
	return out
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

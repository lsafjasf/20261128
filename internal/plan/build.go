package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"clipmerge/internal/conflict"
	"clipmerge/internal/manifest"
)

// Build turns conflict groups into a deterministic remediation plan.
func Build(m *manifest.Manifest, rep *conflict.Report) *Plan {
	p := &Plan{}
	clipsByID := map[string]*manifest.Clip{}
	for i := range m.Clips {
		clipsByID[m.Clips[i].ID] = &m.Clips[i]
	}

	used := map[string]bool{}
	for _, g := range rep.Groups {
		switch g.Kind {
		case conflict.KindNamespaceCollision:
			p.addCrossDayRenames(g, clipsByID, used)
		case conflict.KindFullKeyDuplicate:
			p.addIntraDayRenames(g, clipsByID, used)
		case conflict.KindWrongLock:
			for _, wl := range g.WrongLocks {
				p.Rollback = append(p.Rollback, RollbackEntry{
					ClipID:       wl.ClipID,
					ShootDay:     wl.ShootDay,
					FromUID:      wl.LockedUID,
					FromHash:     wl.LockedHash,
					ToUID:        wl.CorrectUID,
					ToHash:       wl.CorrectHash,
					ResolvedBy:   wl.ResolvedBy,
					Reason:       wl.Reason,
					ConflictKind: g.Kind,
				})
			}
		case conflict.KindRollbackBlocked:
			for _, wl := range g.WrongLocks {
				p.Blocked = append(p.Blocked, BlockedEntry{ClipID: wl.ClipID, Reason: wl.Reason})
			}
		}
	}
	slices.SortFunc(p.Renames, func(a, b RenameEntry) int { return cmp(a.ClipID, b.ClipID) })
	slices.SortFunc(p.Rollback, func(a, b RollbackEntry) int { return cmp(a.ClipID, b.ClipID) })
	slices.SortFunc(p.Blocked, func(a, b BlockedEntry) int { return cmp(a.ClipID, b.ClipID) })
	return p
}

func (p *Plan) addCrossDayRenames(g conflict.Group, byID map[string]*manifest.Clip, used map[string]bool) {
	for _, clipID := range g.ClipIDs {
		c := byID[clipID]
		if c == nil {
			continue
		}
		newClipNo := c.ClipNo + "_" + sanitize(c.ShootDay)
		if key := c.Bin + "/" + newClipNo; used[key] {
			newClipNo = newClipNo + "_" + shortHash(c.ID)
		}
		used[c.Bin+"/"+newClipNo] = true
		p.Renames = append(p.Renames, RenameEntry{
			ClipID:       c.ID,
			ShootDay:     c.ShootDay,
			OldBin:       c.Bin,
			OldClipNo:    c.ClipNo,
			NewBin:       c.Bin,
			NewClipNo:    newClipNo,
			Reason:       "disambiguate same bin+clipNo used on different shoot days",
			ConflictKind: g.Kind,
		})
	}
}

func (p *Plan) addIntraDayRenames(g conflict.Group, byID map[string]*manifest.Clip, used map[string]bool) {
	ids := append([]string(nil), g.ClipIDs...)
	slices.Sort(ids)
	for i, clipID := range ids {
		c := byID[clipID]
		if c == nil {
			continue
		}
		newClipNo := c.ClipNo
		if i > 0 {
			newClipNo = c.ClipNo + "_dup" + shortHash(c.ID)
			for used[c.Bin+"/"+newClipNo] {
				newClipNo = newClipNo + "x"
			}
		}
		used[c.Bin+"/"+newClipNo] = true
		p.Renames = append(p.Renames, RenameEntry{
			ClipID:       c.ID,
			ShootDay:     c.ShootDay,
			OldBin:       c.Bin,
			OldClipNo:    c.ClipNo,
			NewBin:       c.Bin,
			NewClipNo:    newClipNo,
			Reason:       "disambiguate duplicated bin+clipNo within the same shoot day",
			ConflictKind: g.Kind,
		})
	}
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, s)
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

func cmp(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

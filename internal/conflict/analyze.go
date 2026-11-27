package conflict

import (
	"fmt"
	"path/filepath"
	"slices"

	"clipmerge/internal/identity"
	"clipmerge/internal/manifest"
)

// Analyze runs the three-layer identity check and returns conflict groups.
// mediaRoot enables on-disk hash verification; empty means registry-only.
func Analyze(m *manifest.Manifest, mediaRoot string) *Report {
	d := &detector{
		idx:       identity.Build(m),
		mediaRoot: mediaRoot,
		actual:    map[string]string{},
	}
	d.verifyHashes()
	d.scanNamespaceLayer()
	d.scanUIDLayer()
	d.scanHashLayer()
	d.scanLocks()
	d.assignIDs()
	return &Report{Groups: d.groups, ActualHashes: d.actual}
}

func (d *detector) verifyHashes() {
	if d.mediaRoot == "" {
		return
	}
	for _, md := range d.idx.MediaByUID {
		if md.HashAlgo != "sha256" {
			continue
		}
		sum, err := identity.SHA256File(filepath.Join(d.mediaRoot, md.Path))
		if err != nil {
			continue // missing/unreadable file: reported separately if locks dangle
		}
		d.actual[md.UID] = sum
		if md.Hash != "" && sum != md.Hash {
			d.groups = append(d.groups, Group{
				Kind:      KindHashDrift,
				Severity:  SeverityCritical,
				Reason:    fmt.Sprintf("media %s declared hash %s but file %s hashes to %s", md.UID, md.Hash, md.Path, sum),
				MediaUIDs: []string{md.UID},
				Hashes:    []string{md.Hash, sum},
			})
		}
	}
}

func (d *detector) scanNamespaceLayer() {
	for key, clipIDs := range d.idx.ByNamespace {
		if len(clipIDs) < 2 {
			continue
		}
		days := map[string]bool{}
		for _, id := range clipIDs {
			days[d.idx.ClipsByID[id].ShootDay] = true
		}
		kind := KindFullKeyDuplicate
		severity := SeverityCritical
		reason := "same bin+clipNo reused within one shoot day"
		if len(days) > 1 {
			kind = KindNamespaceCollision
			reason = "same bin+clipNo reused across different shoot days after merge"
		}
		g := Group{
			Kind:      kind,
			Severity:  severity,
			Reason:    reason,
			Namespace: key.Bin + "/" + key.ClipNo,
			ClipIDs:   sorted(clipIDs),
		}
		d.groups = append(d.groups, g)
	}
}

func (d *detector) scanUIDLayer() {
	for uid, clipIDs := range d.idx.ByUID {
		if len(clipIDs) < 2 {
			continue
		}
		days := map[string]bool{}
		for _, id := range clipIDs {
			days[d.idx.ClipsByID[id].ShootDay] = true
		}
		if len(days) > 1 {
			d.groups = append(d.groups, Group{
				Kind:     KindUIDReuse,
				Severity: SeverityCritical,
				Reason:   "media UID is locked by clips from different shoot days",
				UIDs:     []string{uid},
				ClipIDs:  sorted(clipIDs),
			})
		} else {
			d.groups = append(d.groups, Group{
				Kind:     KindUIDSplit,
				Severity: SeverityWarning,
				Reason:   "multiple clips from the same shoot day lock the same media UID",
				UIDs:     []string{uid},
				ClipIDs:  sorted(clipIDs),
			})
		}
	}
}

func (d *detector) scanHashLayer() {
	for hash, media := range d.idx.MediaByHash {
		if len(media) < 2 {
			continue
		}
		uidSet := map[string]bool{}
		var uids []string
		for _, md := range media {
			if !uidSet[md.UID] {
				uidSet[md.UID] = true
				uids = append(uids, md.UID)
			}
		}
		if len(uids) < 2 {
			continue
		}
		d.groups = append(d.groups, Group{
			Kind:      KindHashAlias,
			Severity:  SeverityWarning,
			Reason:    "distinct media UIDs resolve to identical content (true duplicates)",
			Hashes:    []string{hash},
			MediaUIDs: sorted(uids),
		})
	}
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	slices.Sort(out)
	return out
}

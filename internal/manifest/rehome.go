package manifest

import (
	"path"
	"sort"
)

// reserveHomes assigns current paths to manifest entries that moved. It
// processes sidecar relationships first and prefers homes where the media and
// sidecar share a directory. The returned used set marks every current path
// already claimed so generic matching cannot double-allocate it.
func reserveHomes(m *Manifest, byHash map[string][]string) (map[string]string, map[string]bool) {
	reserved := map[string]string{} // manifest path -> current path
	used := map[string]bool{}       // current path already claimed

	claim := func(manifestPath, hash string) (string, bool) {
		for _, home := range byHash[hash] {
			if !used[home] {
				used[home] = true
				reserved[manifestPath] = home
				return home, true
			}
		}
		return "", false
	}

	// Deterministic relationship order.
	rels := append([]Relationship(nil), m.Relationships...)
	sort.Slice(rels, func(i, j int) bool {
		if rels[i].MediaPath != rels[j].MediaPath {
			return rels[i].MediaPath < rels[j].MediaPath
		}
		return rels[i].SidecarPath < rels[j].SidecarPath
	})

	for _, rel := range rels {
		mediaHomes := byHash[rel.MediaHash]
		sideHomes := byHash[rel.SidecarHash]
		paired := false
		for _, mediaHome := range mediaHomes {
			if used[mediaHome] {
				continue
			}
			for _, sideHome := range sideHomes {
				if used[sideHome] {
					continue
				}
				if path.Dir(mediaHome) == path.Dir(sideHome) {
					used[mediaHome] = true
					used[sideHome] = true
					reserved[rel.MediaPath] = mediaHome
					reserved[rel.SidecarPath] = sideHome
					paired = true
					break
				}
			}
			if paired {
				break
			}
		}
		if !paired {
			// Keep one-to-one matching even when colocation is impossible.
			claim(rel.MediaPath, rel.MediaHash)
			claim(rel.SidecarPath, rel.SidecarHash)
		}
	}
	return reserved, used
}

// checkColocation verifies sidecar and media still share a directory using
// one-to-one counts per current directory.
func checkColocation(m *Manifest, result *VerifyResult) {
	location := map[string]string{} // manifest path -> current path
	for _, p := range result.Intact {
		location[p] = p
	}
	for _, r := range result.Rehomed {
		location[r.OldPath] = r.NewPath
	}
	currentDir := map[string]string{}
	for oldPath, newPath := range location {
		currentDir[oldPath] = path.Dir(newPath)
	}

	mediaByDir := map[string]int{}
	for _, rel := range m.Relationships {
		dir, ok := currentDir[rel.MediaPath]
		if !ok {
			continue
		}
		if _, ok := currentDir[rel.SidecarPath]; !ok {
			continue
		}
		mediaByDir[dir]++
	}
	for _, rel := range m.Relationships {
		sideDir, ok := currentDir[rel.SidecarPath]
		if !ok {
			continue
		}
		if mediaByDir[sideDir] == 0 {
			result.ColocationIssue = append(result.ColocationIssue, Issue{
				Code:   "sidecar_separated",
				Path:   location[rel.SidecarPath],
				Detail: "no paired media remains colocated with this sidecar",
			})
			continue
		}
		mediaByDir[sideDir]--
	}
}

func sortResult(r *VerifyResult) {
	sort.Strings(r.Intact)
	sort.Strings(r.Missing)
	sort.Strings(r.Extra)
	sort.Slice(r.Rehomed, func(i, j int) bool {
		return r.Rehomed[i].OldPath < r.Rehomed[j].OldPath
	})
	sort.Slice(r.Corrupt, func(i, j int) bool {
		return r.Corrupt[i].Path < r.Corrupt[j].Path
	})
	sort.Slice(r.ColocationIssue, func(i, j int) bool {
		return r.ColocationIssue[i].Path < r.ColocationIssue[j].Path
	})
	sort.Slice(r.Issues, func(i, j int) bool {
		return r.Issues[i].Code < r.Issues[j].Code
	})
}

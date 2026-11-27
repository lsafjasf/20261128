package manifest

import (
	"path"
	"sort"

	"lto-manifest/internal/classify"
)

// buildRelationships pairs each sidecar with same-directory media. A sidecar
// either repeats the full media base name (clip.mp4 -> clip.mp4.srt) or shares
// the extension-less stem (photo.CR2 -> photo.xmp).
func buildRelationships(m *Manifest) {
	// Index media entries by directory, full base name and stem.
	byName := map[string]map[string][]int{}
	byStem := map[string]map[string][]int{}
	for i, e := range m.Entries {
		if e.Class != classify.ClassMedia {
			continue
		}
		dir := path.Dir(e.Path)
		if byName[dir] == nil {
			byName[dir] = map[string][]int{}
			byStem[dir] = map[string][]int{}
		}
		base := path.Base(e.Path)
		byName[dir][base] = append(byName[dir][base], i)
		byStem[dir][classify.Stem(base)] = append(byStem[dir][classify.Stem(base)], i)
	}

	for _, sidecar := range m.Entries {
		if sidecar.Class != classify.ClassSidecar {
			continue
		}
		dir := path.Dir(sidecar.Path)
		stem := classify.Stem(path.Base(sidecar.Path))
		nameIndex, stemIndex := byName[dir], byStem[dir]
		if nameIndex == nil {
			m.Issues = append(m.Issues, Issue{Code: "orphan_sidecar", Path: sidecar.Path,
				Detail: "no media file in same directory with base name " + stem})
			continue
		}
		// Prefer the exact full-name match, then fall back to shared stem.
		candidates := nameIndex[stem]
		if candidates == nil {
			candidates = stemIndex[stem]
		}
		switch len(candidates) {
		case 0:
			m.Issues = append(m.Issues, Issue{Code: "orphan_sidecar", Path: sidecar.Path,
				Detail: "no media file named " + stem + " in " + dir})
		case 1:
			media := m.Entries[candidates[0]]
			m.Relationships = append(m.Relationships, Relationship{
				MediaPath:   media.Path,
				SidecarPath: sidecar.Path,
				MediaHash:   media.SHA256,
				SidecarHash: sidecar.SHA256,
				Rule:        "same-directory basename match",
			})
		default:
			paths := make([]string, 0, len(candidates))
			for _, idx := range candidates {
				paths = append(paths, m.Entries[idx].Path)
			}
			m.Issues = append(m.Issues, Issue{Code: "ambiguous_sidecar", Path: sidecar.Path,
				Detail: "multiple media candidates: " + joinPaths(paths)})
		}
	}

	sort.Slice(m.Relationships, func(i, j int) bool {
		if m.Relationships[i].SidecarPath != m.Relationships[j].SidecarPath {
			return m.Relationships[i].SidecarPath < m.Relationships[j].SidecarPath
		}
		return m.Relationships[i].MediaPath < m.Relationships[j].MediaPath
	})
}

// buildDuplicates groups entries by content hash and computes wasted bytes.
func buildDuplicates(m *Manifest) {
	groups := map[string][]int{}
	for i, e := range m.Entries {
		groups[e.SHA256] = append(groups[e.SHA256], i)
	}
	hashes := make([]string, 0, len(groups))
	for sum, indexes := range groups {
		if len(indexes) > 1 {
			hashes = append(hashes, sum)
		}
	}
	sort.Strings(hashes)
	for _, sum := range hashes {
		indexes := groups[sum]
		paths := make([]string, 0, len(indexes))
		var size int64
		for _, idx := range indexes {
			paths = append(paths, m.Entries[idx].Path)
			size = m.Entries[idx].Size
		}
		sort.Strings(paths)
		m.Duplicates = append(m.Duplicates, DuplicateGroup{
			SHA256: sum,
			Size:   size,
			Paths:  paths,
			Wasted: size * int64(len(paths)-1),
		})
	}
}

// buildBudget compares total and de-duplicated footprint with the limit.
func buildBudget(m *Manifest, limit int64) {
	unique := map[string]int64{}
	var reclaimable int64
	for _, d := range m.Duplicates {
		reclaimable += d.Wasted
	}
	for _, e := range m.Entries {
		unique[e.SHA256] = e.Size
	}
	var uniqueSize int64
	for _, size := range unique {
		uniqueSize += size
	}
	m.Budget = &Budget{
		Limit:       limit,
		TotalSize:   m.TotalSize,
		UniqueSize:  uniqueSize,
		Reclaimable: reclaimable,
		Over:        m.TotalSize > limit,
		Ratio:       float64(m.TotalSize) / float64(limit),
	}
}

func joinPaths(paths []string) string {
	out := ""
	for i, p := range paths {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

package dedup

import (
	"sort"

	"mediadedup/media"
)

// Group is a deduplication candidate group: assets that all depict the same
// underlying shot. RelationDifferent pairs never land in the same group.
type Group struct {
	ID      string
	Members []media.Asset
	// MasterID is the user-selected (or recommended) primary asset.
	MasterID string
	// Relations stores pair verdicts keyed by "a|b" with sorted IDs.
	Relations map[string]PairResult
}

// HasMember reports whether id belongs to this group.
func (g *Group) HasMember(id string) bool {
	for _, a := range g.Members {
		if a.ID == id {
			return true
		}
	}
	return false
}

// MemberRelation returns the classified relation between two group members.
func (g *Group) MemberRelation(a, b string) (PairResult, bool) {
	r, ok := g.Relations[pairKey(a, b)]
	return r, ok
}

// Master returns the selected primary asset, if set and still present.
func (g *Group) Master() (media.Asset, bool) {
	for _, a := range g.Members {
		if a.ID == g.MasterID {
			return a, true
		}
	}
	return media.Asset{}, false
}

// SetMaster assigns the primary asset. The ID must belong to the group.
func (g *Group) SetMaster(id string) error {
	if !g.HasMember(id) {
		return errMasterNotInGroup(id, g.ID)
	}
	g.MasterID = id
	return nil
}

// Pool is the analyzed media pool.
type Pool struct {
	Assets     []media.Asset
	Groups     []*Group
	Thresholds Thresholds
}

// GroupOf returns the group containing the asset, if any.
func (p *Pool) GroupOf(assetID string) (*Group, bool) {
	for _, g := range p.Groups {
		if g.HasMember(assetID) {
			return g, true
		}
	}
	return nil, false
}

// unionFind is a standard disjoint-set structure.
type unionFind struct {
	parent map[string]string
	rank   map[string]int
}

func newUnionFind(ids []string) *unionFind {
	uf := &unionFind{parent: make(map[string]string, len(ids)), rank: make(map[string]int, len(ids))}
	for _, id := range ids {
		uf.parent[id] = id
	}
	return uf
}

func (uf *unionFind) find(x string) string {
	for uf.parent[x] != x {
		uf.parent[x] = uf.parent[uf.parent[x]]
		x = uf.parent[x]
	}
	return x
}

func (uf *unionFind) union(a, b string) {
	ra, rb := uf.find(a), uf.find(b)
	if ra == rb {
		return
	}
	if uf.rank[ra] < uf.rank[rb] {
		ra, rb = rb, ra
	}
	uf.parent[rb] = ra
	if uf.rank[ra] == uf.rank[rb] {
		uf.rank[ra]++
	}
}

// Build analyzes every candidate pair and assembles deduplication groups.
//
// Only assets within DurationTol of each other are ever compared, and only
// RelationIdentical / RelationTranscode pairs merge components. Singletons
// (assets with no duplicate) are omitted from Groups and returned separately.
func Build(assets []media.Asset, t Thresholds) *Pool {
	pool := &Pool{Assets: assets, Thresholds: t}
	if len(assets) == 0 {
		return pool
	}

	byID := make(map[string]media.Asset, len(assets))
	ids := make([]string, len(assets))
	for i, a := range assets {
		byID[a.ID] = a
		ids[i] = a.ID
	}

	uf := newUnionFind(ids)
	verdicts := make(map[string]PairResult)

	for _, pair := range candidatePairs(assets, t.DurationTol) {
		a, b := byID[pair[0]], byID[pair[1]]
		res := ClassifyPair(a, b, t)
		verdicts[pairKey(a.ID, b.ID)] = res
		if res.Relation == RelationIdentical || res.Relation == RelationTranscode {
			uf.union(a.ID, b.ID)
		}
	}

	components := make(map[string][]string)
	for _, id := range ids {
		root := uf.find(id)
		components[root] = append(components[root], id)
	}

	roots := make([]string, 0, len(components))
	for root, members := range components {
		if len(members) > 1 {
			roots = append(roots, root)
		}
	}
	sort.Strings(roots)

	for gi, root := range roots {
		memberIDs := components[root]
		sort.Strings(memberIDs)
		g := &Group{
			ID:        groupName(gi),
			Relations: make(map[string]PairResult),
		}
		for _, id := range memberIDs {
			g.Members = append(g.Members, byID[id])
		}
		for i := 0; i < len(memberIDs); i++ {
			for j := i + 1; j < len(memberIDs); j++ {
				key := pairKey(memberIDs[i], memberIDs[j])
				if r, ok := verdicts[key]; ok {
					g.Relations[key] = r
				}
			}
		}
		g.MasterID = RecommendMaster(g)
		pool.Groups = append(pool.Groups, g)
	}
	return pool
}

// candidatePairs returns ID pairs whose duration gap is within tolerance.
// Assets are sorted by duration and swept with a sliding window, avoiding an
// O(n^2) full comparison pass on large pools.
func candidatePairs(assets []media.Asset, tol float64) [][2]string {
	sorted := make([]media.Asset, len(assets))
	copy(sorted, assets)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Duration != sorted[j].Duration {
			return sorted[i].Duration < sorted[j].Duration
		}
		return sorted[i].ID < sorted[j].ID
	})

	var pairs [][2]string
	left := 0
	for right := 0; right < len(sorted); right++ {
		for sorted[right].Duration-sorted[left].Duration > tol {
			left++
		}
		for k := left; k < right; k++ {
			pairs = append(pairs, [2]string{sorted[k].ID, sorted[right].ID})
		}
	}
	return pairs
}

// RecommendMaster picks the highest-fidelity member: upright pixel count first,
// then frame rate, then unrotated storage (fewer transforms on replace), then
// bitrate, then stable ID ordering for determinism.
func RecommendMaster(g *Group) string {
	best := g.Members[0]
	for _, a := range g.Members[1:] {
		if masterScore(a) > masterScore(best) {
			best = a
		}
	}
	return best.ID
}

func masterScore(a media.Asset) float64 {
	rotPenalty := 0.0
	if a.Rotation != media.Rotate0 {
		rotPenalty = 0.5
	}
	return float64(a.UprightWidth()*a.UprightHeight()) +
		a.FrameRate +
		float64(a.BitrateK)/1_000_000.0 -
		rotPenalty
}

func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

func groupName(i int) string {
	return "G" + strconvI(i+1)
}

func strconvI(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

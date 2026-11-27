package identity

import "clipmerge/internal/manifest"

// NamespaceKey is the editing identity layer: bin name + clip number.
type NamespaceKey struct {
	Bin    string
	ClipNo string
}

// Index holds the three identity layers of the merged project:
// namespace (bin+clipNo), media UID, and content hash.
type Index struct {
	ByNamespace map[NamespaceKey][]string // namespace layer -> clip IDs
	ByUID       map[string][]string       // UID layer -> media UIDs -> clip IDs
	ByHash      map[string][]string       // hash layer -> clip IDs

	MediaByUID  map[string]*manifest.Media
	MediaByHash map[string][]*manifest.Media
	ClipsByID   map[string]*manifest.Clip
}

// Build constructs all three identity indexes.
func Build(m *manifest.Manifest) *Index {
	idx := &Index{
		ByNamespace: map[NamespaceKey][]string{},
		ByUID:       map[string][]string{},
		ByHash:      map[string][]string{},
		MediaByUID:  map[string]*manifest.Media{},
		MediaByHash: map[string][]*manifest.Media{},
		ClipsByID:   map[string]*manifest.Clip{},
	}
	for i := range m.Media {
		md := &m.Media[i]
		idx.MediaByUID[md.UID] = md
		if md.Hash != "" {
			idx.MediaByHash[md.Hash] = append(idx.MediaByHash[md.Hash], md)
		}
	}
	for i := range m.Clips {
		c := &m.Clips[i]
		idx.ClipsByID[c.ID] = c
		key := NamespaceKey{Bin: c.Bin, ClipNo: c.ClipNo}
		idx.ByNamespace[key] = append(idx.ByNamespace[key], c.ID)
		idx.ByUID[c.LockedUID] = append(idx.ByUID[c.LockedUID], c.ID)
		if c.LockedHash != "" {
			idx.ByHash[c.LockedHash] = append(idx.ByHash[c.LockedHash], c.ID)
		}
	}
	return idx
}

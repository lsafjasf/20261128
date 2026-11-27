package plan

import (
	"encoding/json"
	"fmt"
	"os"

	"clipmerge/internal/manifest"
)

// Apply produces a corrected manifest copy: rollback entries rewrite the clip
// locks, rename entries rewrite bin/clipNo. The input manifest is never
// modified in place; blocked rollbacks are left untouched.
func Apply(m *manifest.Manifest, p *Plan, outputPath string) (int, int, error) {
	cp := *m
	cp.Clips = append([]manifest.Clip(nil), m.Clips...)

	rollbacks := map[string]RollbackEntry{}
	for _, e := range p.Rollback {
		rollbacks[e.ClipID] = e
	}
	renames := map[string]RenameEntry{}
	for _, e := range p.Renames {
		renames[e.ClipID] = e
	}

	rolled, renamed := 0, 0
	for i := range cp.Clips {
		c := &cp.Clips[i]
		if e, ok := rollbacks[c.ID]; ok {
			c.LockedUID = e.ToUID
			c.LockedHash = e.ToHash
			c.ExpectedUID = ""
			rolled++
		}
		if e, ok := renames[c.ID]; ok && e.NewClipNo != e.OldClipNo {
			c.Bin = e.NewBin
			c.ClipNo = e.NewClipNo
			renamed++
		}
	}

	raw, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return 0, 0, fmt.Errorf("encode corrected manifest: %w", err)
	}
	if err := os.WriteFile(outputPath, append(raw, '\n'), 0o644); err != nil {
		return 0, 0, fmt.Errorf("write corrected manifest: %w", err)
	}
	return rolled, renamed, nil
}

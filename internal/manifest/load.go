package manifest

import (
	"encoding/json"
	"fmt"
	"os"
)

// Load reads and validates a merged project manifest.
func Load(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks structural prerequisites; identity-level conflicts are
// detected later by the conflict package.
func (m *Manifest) Validate() error {
	var problems []string
	seenUID := map[string]bool{}
	seenPath := map[string]bool{}
	for i, md := range m.Media {
		at := fmt.Sprintf("media[%d]", i)
		if md.UID == "" {
			problems = append(problems, at+" missing uid")
		} else if seenUID[md.UID] {
			problems = append(problems, at+" duplicate uid "+md.UID)
		} else {
			seenUID[md.UID] = true
		}
		if md.Path == "" {
			problems = append(problems, at+" missing path")
		} else if seenPath[md.Path] {
			problems = append(problems, at+" duplicate path "+md.Path)
		} else {
			seenPath[md.Path] = true
		}
		if md.ShootDay == "" {
			problems = append(problems, at+" ("+md.UID+") missing shootDay")
		}
		if md.Hash != "" && md.HashAlgo != "sha256" {
			problems = append(problems, at+" unsupported hashAlgo "+md.HashAlgo)
		}
	}
	for i, c := range m.Clips {
		at := fmt.Sprintf("clips[%d]", i)
		if c.ID == "" {
			problems = append(problems, at+" missing id")
		}
		if c.Bin == "" || c.ClipNo == "" {
			problems = append(problems, at+" ("+c.ID+") missing bin/clipNo")
		}
		if c.ShootDay == "" {
			problems = append(problems, at+" ("+c.ID+") missing shootDay")
		}
		if c.LockedUID == "" {
			problems = append(problems, at+" ("+c.ID+") missing lockedUID")
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid manifest: %v", problems)
	}
	return nil
}

package project

import (
	"encoding/json"
	"fmt"
	"os"
)

// Load reads a project from a JSON file.
func Load(path string) (*Project, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load project %s: %w", path, err)
	}
	var p Project
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse project %s: %w", path, err)
	}
	if p.Clips == nil {
		p.Clips = make(map[string]*Clip)
	}
	if p.Media == nil {
		p.Media = make(map[string]*MediaRef)
	}
	return &p, nil
}

// Save writes the project as indented JSON.
func Save(path string, p *Project) error {
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal project: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("save project %s: %w", path, err)
	}
	return nil
}

package batchout

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func validateProvenance(p *Provenance) error {
	if p.Version != Version {
		return fmt.Errorf("unsupported provenance version %d", p.Version)
	}
	if err := validateSHA256("task id", p.TaskID); err != nil {
		return err
	}
	if !utf8.ValidString(p.Recipe) {
		return errors.New("recipe is not valid UTF-8")
	}
	for key, value := range p.Params {
		if key == "" || !utf8.ValidString(key) || !utf8.ValidString(value) {
			return errors.New("parameter is not valid UTF-8")
		}
	}
	if p.AttemptID == "" || strings.ContainsAny(p.AttemptID, `/\`) || strings.Contains(p.AttemptID, "..") {
		return errors.New("unsafe attempt id")
	}
	if err := validateSHA256("params digest", p.ParamsDigest); err != nil {
		return err
	}
	for _, input := range p.Inputs {
		if input.Name == "" {
			return errors.New("input has no name")
		}
		if !utf8.ValidString(input.Name) || !utf8.ValidString(input.Path) {
			return errors.New("input metadata is not valid UTF-8")
		}
		if input.Size < 0 {
			return errors.New("input has a negative size")
		}
		if err := validateSHA256("input digest", input.SHA256); err != nil {
			return err
		}
	}
	seen := make(map[string]struct{}, len(p.Outputs))
	for _, output := range p.Outputs {
		name, err := cleanRelative(output.Name)
		if err != nil {
			return fmt.Errorf("invalid output name %q: %w", output.Name, err)
		}
		if name != output.Name {
			return fmt.Errorf("output name is not canonical: %q", output.Name)
		}
		path, err := cleanRelative(output.Path)
		if err != nil {
			return fmt.Errorf("invalid output path %q: %w", output.Path, err)
		}
		if path != output.Path {
			return fmt.Errorf("output path is not canonical: %q", output.Path)
		}
		if _, exists := seen[output.Path]; exists {
			return fmt.Errorf("duplicate output path %q", output.Path)
		}
		seen[output.Path] = struct{}{}
		if output.Size < 0 {
			return fmt.Errorf("output %q has a negative size", output.Path)
		}
		if err := validateSHA256("output digest", output.SHA256); err != nil {
			return fmt.Errorf("output %q: %w", output.Path, err)
		}
	}
	return nil
}

func validateJournal(j *journal) error {
	if err := validateProvenance(&j.Provenance); err != nil {
		return err
	}
	expectedStage := filepath.ToSlash(filepath.Join(stateRoot, stateTmp, j.TaskID, j.AttemptID))
	if j.StageDir != expectedStage {
		return fmt.Errorf("unexpected staging directory %q", j.StageDir)
	}
	switch j.State {
	case StateStaging:
		if len(j.Outputs) != 0 {
			return errors.New("staging journal unexpectedly contains outputs")
		}
	case StatePrepared, StateCommitting, StateCommitted:
		if len(j.Outputs) == 0 {
			return fmt.Errorf("%s journal contains no outputs", j.State)
		}
	case StateAborted:
	default:
		return fmt.Errorf("unknown journal state %q", j.State)
	}
	return nil
}

func validateSHA256(label, value string) error {
	if len(value) != 64 {
		return fmt.Errorf("%s is not a SHA-256: %q", label, value)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("%s is not hexadecimal: %q", label, value)
	}
	return nil
}

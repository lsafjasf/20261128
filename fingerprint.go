package batchout

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"
)

type taskIdentity struct {
	Version      int                 `json:"version"`
	Recipe       string              `json:"recipe"`
	Inputs       []taskIdentityInput `json:"inputs"`
	ParamsDigest string              `json:"params_digest"`
}

type taskIdentityInput struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func fingerprint(spec TaskSpec) (string, []InputRecord, string, map[string]string, error) {
	if len(spec.Inputs) == 0 {
		return "", nil, "", nil, errors.New("batchout: at least one input is required")
	}
	if !utf8.ValidString(spec.Recipe) {
		return "", nil, "", nil, errors.New("batchout: recipe is not valid UTF-8")
	}

	params := make(map[string]string, len(spec.Params))
	for key, value := range spec.Params {
		if key == "" {
			return "", nil, "", nil, errors.New("batchout: parameter name must not be empty")
		}
		if !utf8.ValidString(key) || !utf8.ValidString(value) {
			return "", nil, "", nil, errors.New("batchout: parameter is not valid UTF-8")
		}
		params[key] = value
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return "", nil, "", nil, err
	}
	paramsSum := sha256.Sum256(paramsJSON)
	paramsDigest := hex.EncodeToString(paramsSum[:])

	records := make([]InputRecord, 0, len(spec.Inputs))
	seen := make(map[string]struct{}, len(spec.Inputs))
	for _, input := range spec.Inputs {
		if input.Name == "" {
			return "", nil, "", nil, errors.New("batchout: input name must not be empty")
		}
		if !utf8.ValidString(input.Name) {
			return "", nil, "", nil, fmt.Errorf("batchout: input name %q is not valid UTF-8", input.Name)
		}
		if _, exists := seen[input.Name]; exists {
			return "", nil, "", nil, fmt.Errorf("batchout: duplicate input name %q", input.Name)
		}
		seen[input.Name] = struct{}{}
		if input.Path == "" {
			return "", nil, "", nil, fmt.Errorf("batchout: input %q has no path", input.Name)
		}
		if !utf8.ValidString(input.Path) {
			return "", nil, "", nil, fmt.Errorf("batchout: input %q path is not valid UTF-8", input.Name)
		}
		abs, err := filepath.Abs(input.Path)
		if err != nil {
			return "", nil, "", nil, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return "", nil, "", nil, fmt.Errorf("stat input %q: %w", input.Name, err)
		}
		if !info.Mode().IsRegular() {
			return "", nil, "", nil, fmt.Errorf("batchout: input %q is not a regular file", input.Name)
		}
		size, digest, err := fileDigest(abs)
		if err != nil {
			return "", nil, "", nil, fmt.Errorf("hash input %q: %w", input.Name, err)
		}
		records = append(records, InputRecord{
			Name:   input.Name,
			Path:   abs,
			Size:   size,
			SHA256: digest,
		})
	}

	identity := taskIdentity{
		Version:      Version,
		Recipe:       spec.Recipe,
		Inputs:       make([]taskIdentityInput, 0, len(records)),
		ParamsDigest: paramsDigest,
	}
	for _, input := range records {
		identity.Inputs = append(identity.Inputs, taskIdentityInput{
			Name:   input.Name,
			Size:   input.Size,
			SHA256: input.SHA256,
		})
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return "", nil, "", nil, err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), records, paramsDigest, params, nil
}

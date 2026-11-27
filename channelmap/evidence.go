package channelmap

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	MappingFileName  = "mapping.json"
	MappingCSVName   = "mapping.csv"
	ReadbackFileName = "readback.json"
)

// ReadbackRow is one independently reconstructed routing row.
type ReadbackRow struct {
	SourceIndex      int   `json:"source_index"`
	SourcePosition   int   `json:"source_position"`
	SourceLabel      Label `json:"source_label"`
	DeliveryPosition int   `json:"delivery_position"`
	DeliveryLabel    Label `json:"delivery_label"`
}

// ReadbackCheck is one named invariant verified from disk.
type ReadbackCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// ReadbackEvidence proves that the written mapping parses back to a complete,
// non-overlapping bijection and preserves both encoded and delivery order.
type ReadbackEvidence struct {
	SchemaVersion         int             `json:"schema_version"`
	MappingFile           string          `json:"mapping_file"`
	MappingSHA256         string          `json:"mapping_sha256"`
	Verified              bool            `json:"verified"`
	SourceEncodedOrder    []Label         `json:"source_encoded_order"`
	DeliveryReadbackOrder []Label         `json:"delivery_readback_order"`
	Rows                  []ReadbackRow   `json:"rows"`
	Checks                []ReadbackCheck `json:"checks"`
}

// MarshalPlan returns the canonical JSON representation stored as evidence.
func MarshalPlan(plan *Plan) ([]byte, error) {
	if plan == nil {
		return nil, fmt.Errorf("mapping plan is nil")
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode mapping plan: %w", err)
	}
	return append(data, '\n'), nil
}

// WriteEvidence writes mapping.json, mapping.csv, and readback.json. The
// readback is produced by reading mapping.json back from disk, not by reusing
// the in-memory plan.
func WriteEvidence(dir string, plan *Plan) (*ReadbackEvidence, error) {
	if plan == nil {
		return nil, fmt.Errorf("mapping plan is nil")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create evidence directory: %w", err)
	}
	mappingBytes, err := MarshalPlan(plan)
	if err != nil {
		return nil, err
	}
	mappingPath := filepath.Join(dir, MappingFileName)
	if err := os.WriteFile(mappingPath, mappingBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", mappingPath, err)
	}
	if err := writeMappingCSV(filepath.Join(dir, MappingCSVName), plan); err != nil {
		return nil, err
	}

	written, err := os.ReadFile(mappingPath)
	if err != nil {
		return nil, fmt.Errorf("read back %s: %w", mappingPath, err)
	}
	evidence := VerifyReadback(written, plan, MappingFileName)
	readbackBytes, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode readback evidence: %w", err)
	}
	readbackPath := filepath.Join(dir, ReadbackFileName)
	if err := os.WriteFile(readbackPath, append(readbackBytes, '\n'), 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", readbackPath, err)
	}
	if !evidence.Verified {
		return evidence, fmt.Errorf("mapping readback verification failed")
	}
	return evidence, nil
}

func writeMappingCSV(path string, plan *Plan) error {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	header := []string{
		"delivery_position", "delivery_label",
		"source_position", "source_label", "reordered",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("encode mapping CSV: %w", err)
	}
	for _, row := range plan.Mappings {
		record := []string{
			strconv.Itoa(row.Delivery.Position),
			string(row.Delivery.Label),
			strconv.Itoa(row.Source.Position),
			string(row.Source.Label),
			strconv.FormatBool(row.Reordered),
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("encode mapping CSV: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("encode mapping CSV: %w", err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

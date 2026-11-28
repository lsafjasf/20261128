package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mxfindex"
)

func TestWriteJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	report := &mxfindex.Result{
		Status:  mxfindex.StatusIncomplete,
		Success: false,
		Issues: []mxfindex.Issue{{
			Code:     "index.gap",
			Severity: mxfindex.SeverityError,
			Message:  "missing segment",
		}},
	}
	if err := writeJSON(path, report); err != nil {
		t.Fatalf("writeJSON() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var decoded mxfindex.Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON is invalid: %v", err)
	}
	if decoded.Success || decoded.Status != mxfindex.StatusIncomplete {
		t.Fatalf("decoded result = %+v", decoded)
	}
}

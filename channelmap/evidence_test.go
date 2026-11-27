package channelmap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func reorderedPlan(t *testing.T) *Plan {
	t.Helper()
	layout := testLayout(LabelLeft, LabelRight, LabelCenter, LabelLFE,
		LabelLeftSurround, LabelRightSurround)
	delivery := deliveryFromLabels("broadcast-5.1",
		LabelLeft, LabelRight, LabelLeftSurround, LabelRightSurround,
		LabelCenter, LabelLFE)
	plan, err := BuildPlan(layout, delivery)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	return plan
}

func TestWriteEvidenceAndReadback(t *testing.T) {
	plan := reorderedPlan(t)
	dir := t.TempDir()
	evidence, err := WriteEvidence(dir, plan)
	if err != nil {
		t.Fatalf("WriteEvidence() error = %v", err)
	}
	if !evidence.Verified {
		t.Fatal("readback evidence is not verified")
	}
	if len(evidence.Rows) != 6 {
		t.Fatalf("readback has %d rows, want 6", len(evidence.Rows))
	}
	if evidence.Rows[4].SourceLabel != LabelCenter {
		t.Fatalf("delivery row 5 source = %s, want C", evidence.Rows[4].SourceLabel)
	}
	if evidence.Rows[2].SourceLabel != LabelLeftSurround {
		t.Fatalf("delivery row 3 source = %s, want Ls", evidence.Rows[2].SourceLabel)
	}

	mappingData, err := os.ReadFile(filepath.Join(dir, MappingFileName))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(mappingData)
	if evidence.MappingSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("readback digest = %s, file digest = %x", evidence.MappingSHA256, sum)
	}

	readbackData, err := os.ReadFile(filepath.Join(dir, ReadbackFileName))
	if err != nil {
		t.Fatal(err)
	}
	var persisted ReadbackEvidence
	if err := json.Unmarshal(readbackData, &persisted); err != nil {
		t.Fatalf("decode readback.json: %v", err)
	}
	if !persisted.Verified {
		t.Fatal("persisted readback evidence is not verified")
	}
	for _, check := range persisted.Checks {
		if !check.Passed {
			t.Errorf("readback check %s failed: %s", check.Name, check.Detail)
		}
	}

	csvData, err := os.ReadFile(filepath.Join(dir, MappingCSVName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(csvData), "delivery_position,delivery_label") {
		t.Fatalf("mapping.csv has unexpected header: %s", csvData)
	}
}

func TestVerifyReadbackDetectsTampering(t *testing.T) {
	plan := reorderedPlan(t)
	data, err := MarshalPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	valid := VerifyReadback(data, plan, MappingFileName)
	if !valid.Verified {
		t.Fatal("untampered mapping failed readback")
	}

	tampered := []byte(strings.Replace(string(data), `"label": "C"`, `"label": "LFE"`, 1))
	result := VerifyReadback(tampered, plan, MappingFileName)
	if result.Verified {
		t.Fatal("tampered mapping passed readback")
	}
	if !checkFailed(result, "mapping_matches_validation") {
		t.Fatal("tampered mapping did not fail mapping_matches_validation")
	}
}

func TestVerifyReadbackDetectsIncompleteRows(t *testing.T) {
	plan := reorderedPlan(t)
	data, err := MarshalPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	var mappings []ChannelMapping
	if err := json.Unmarshal(object["mappings"], &mappings); err != nil {
		t.Fatal(err)
	}
	object["mappings"], _ = json.Marshal(mappings[1:])
	incomplete, _ := json.Marshal(object)

	result := VerifyReadback(incomplete, plan, MappingFileName)
	if result.Verified {
		t.Fatal("incomplete mapping passed readback")
	}
	if !checkFailed(result, "delivery_coverage") || !checkFailed(result, "source_coverage") {
		t.Fatalf("expected coverage failures, got %+v", result.Checks)
	}
}

func checkFailed(evidence *ReadbackEvidence, name string) bool {
	for _, check := range evidence.Checks {
		if check.Name == name {
			return !check.Passed
		}
	}
	return false
}

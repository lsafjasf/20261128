package report

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"resampleforensics/internal/analysis"
)

func TestJSONEncodesUnavailableAsNull(t *testing.T) {
	r := analysis.Report{GroupDelay: analysis.GroupDelayReport{RippleMS: math.NaN()}}
	encoded, err := JSON(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"passband_ripple_p95_p5_ms": null`) {
		t.Fatalf("non-finite value was not null:\n%s", encoded)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
}

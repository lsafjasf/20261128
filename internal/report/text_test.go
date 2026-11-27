package report

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"resampleforensics/internal/analysis"
)

func TestTextDoesNotHideUnavailableMetrics(t *testing.T) {
	r := analysis.Report{
		Source:      analysis.StreamInfo{SampleRate: 44100, Samples: 4410, DurationMS: 100},
		Destination: analysis.StreamInfo{SampleRate: 48000, Samples: 4800, DurationMS: 100},
		GroupDelay: analysis.GroupDelayReport{
			RippleMS: math.NaN(),
			Bands: []analysis.BandDelay{{
				Name: "mid", LowHz: 1000, HighHz: 5000, RippleMS: math.NaN(),
			}},
		},
		Boundary: analysis.BoundaryReport{InteriorNRMSE: math.NaN()},
	}
	var buf bytes.Buffer
	if err := Text(&buf, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "n/a") {
		t.Fatalf("unavailable metric was not identified:\n%s", out)
	}
	if !strings.Contains(out, "未使用整段平均频谱相似度") {
		t.Fatalf("method constraint missing:\n%s", out)
	}
}

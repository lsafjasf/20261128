package releaseaudit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderStableMatrixFormats(t *testing.T) {
	report := Audit(completePackage(), Options{})
	markdown, err := report.Render(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| 版本 | 画幅 |", "2.39:1 (4096×1716)", "0 个错误"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown missing %q", want)
		}
	}

	csvText, err := report.Render(FormatCSV)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(csvText, "version,kind,aspectRatio") {
		t.Errorf("unexpected CSV header:\n%s", csvText)
	}

	jsonText, err := report.Render(FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal([]byte(jsonText), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Rows) != 3 {
		t.Fatalf("decoded rows = %d", len(decoded.Rows))
	}
}

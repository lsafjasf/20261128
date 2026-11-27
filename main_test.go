package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRunY4MJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.y4m")
	if err := os.WriteFile(path, syntheticY4M(), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"-input", path, "-format", "y4m", "-output-format", "jsonl"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var frames, summaries int
	var summary map[string]any
	sc := bufio.NewScanner(&stdout)
	for sc.Scan() {
		var obj map[string]any
		if err := json.Unmarshal(sc.Bytes(), &obj); err != nil {
			t.Fatalf("invalid json line: %v: %s", err, sc.Text())
		}
		switch obj["type"] {
		case "frame":
			frames++
			if obj["evidence"] == nil {
				t.Fatal("frame record has no evidence")
			}
		case "summary":
			summaries++
			summary = obj
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if frames != 3 || summaries != 1 {
		t.Fatalf("frames=%d summaries=%d", frames, summaries)
	}
	if int(summary["frames"].(float64)) != frames {
		t.Fatalf("summary frames=%v, records=%d", summary["frames"], frames)
	}
}

func TestRunJSONDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.y4m")
	if err := os.WriteFile(path, syntheticY4M(), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"-i", path, "-output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var doc struct {
		Type   string `json:"type"`
		Frames []any  `json:"frames"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Type != "document" || len(doc.Frames) != 3 {
		t.Fatalf("type=%s frames=%d", doc.Type, len(doc.Frames))
	}
}

func TestRunRawYUV(t *testing.T) {
	const w, h = 32, 16
	var raw bytes.Buffer
	for i := 0; i < 2; i++ {
		for p := 0; p < w*h; p++ {
			raw.WriteByte(byte(20 + p%200))
		}
		for p := 0; p < (w/2)*(h/2)*2; p++ {
			raw.WriteByte(128)
		}
	}
	path := filepath.Join(t.TempDir(), "sample.yuv")
	if err := os.WriteFile(path, raw.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-i", path, "-format", "raw", "-size", fmt.Sprintf("%dx%d", w, h),
		"-fps", "29.97", "-output-format", "jsonl",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if got := bytes.Count(stdout.Bytes(), []byte(`"type":"frame"`)); got != 2 {
		t.Fatalf("frame records=%d, want 2\n%s", got, stdout.String())
	}
}

func syntheticY4M() []byte {
	const w, h = 64, 32
	var b bytes.Buffer
	fmt.Fprintf(&b, "YUV4MPEG2 W%d H%d F25:1 Ip C420jpeg\n", w, h)
	for i := 0; i < 3; i++ {
		b.WriteString("FRAME\n")
		for row := 0; row < h; row++ {
			pos := 16 + 2*i
			if row%2 == 1 {
				pos += 2
			}
			for x := 0; x < w; x++ {
				if x >= pos && x < pos+12 {
					b.WriteByte(220)
				} else {
					b.WriteByte(24)
				}
			}
		}
		for j := 0; j < 2*(w/2)*(h/2); j++ {
			b.WriteByte(128)
		}
	}
	return b.Bytes()
}

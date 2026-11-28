package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lsafjasf/20261128/internal/ltc"
)

func TestRunWithTwoSources(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.txt")
	bPath := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(aPath, []byte(cliFrame(ltc.Timecode{Hours: 23, Minutes: 59, Seconds: 59, Frames: 29}, true)+"\n"+
		cliFrame(ltc.Timecode{Hours: 0, Minutes: 0, Seconds: 0, Frames: 0}, true)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte(cliFrame(ltc.Timecode{Hours: 23, Minutes: 59, Seconds: 59, Frames: 29}, false)+"\n"+
		cliFrame(ltc.Timecode{Hours: 0, Minutes: 0, Seconds: 0, Frames: 0}, false)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := run([]string{"-fps", "30", "A=" + aPath, "B=" + bPath}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}
	var got ltc.Analysis
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 2 || got.Sources[0].Label != "A" {
		t.Fatalf("sources = %+v", got.Sources)
	}
	if got.Sources[0].Segments[0].DateStatus != ltc.DateDetermined {
		t.Fatalf("A date status = %s", got.Sources[0].Segments[0].DateStatus)
	}
	if got.Sources[1].Segments[0].DateStatus != ltc.DateAbsent ||
		!got.Sources[1].Segments[0].Ambiguous {
		t.Fatalf("B segment = %+v", got.Sources[1].Segments[0])
	}
	if got.CrossSource.Ordering != "ambiguous" {
		t.Fatalf("cross-source = %+v", got.CrossSource)
	}
}

func TestRunAllowsFlagsAfterSources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "one.txt")
	if err := os.WriteFile(path, []byte(cliFrame(ltc.Timecode{Hours: 1, Minutes: 2, Seconds: 3, Frames: 4}, false)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"ONE=" + path, "-fps", "30", "-compact"}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"label":"ONE"`)) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func cliFrame(tc ltc.Timecode, dated bool) string {
	bits := []byte("0000000000000000000000000000000000000000000000000000000000000000" + ltc.SyncBits)
	put := func(pos, width, value int) {
		for i := 0; i < width; i++ {
			if value&(1<<i) != 0 {
				bits[pos+i] = '1'
			}
		}
	}
	put(0, 4, tc.Frames%10)
	put(8, 2, tc.Frames/10)
	put(16, 4, tc.Seconds%10)
	put(24, 3, tc.Seconds/10)
	put(32, 4, tc.Minutes%10)
	put(40, 3, tc.Minutes/10)
	put(48, 4, tc.Hours%10)
	put(56, 2, tc.Hours/10)
	if dated {
		bits[59] = '1'
		dateGroups := [6]int{8, 1, 7, 9, 4, 0}
		if tc.Hours == 0 {
			dateGroups = [6]int{9, 1, 7, 9, 4, 0}
		}
		for i, value := range dateGroups {
			put([]int{4, 12, 20, 28, 36, 44}[i], 4, value)
		}
		bits[60+3] = '1'
	}
	return string(bits)
}

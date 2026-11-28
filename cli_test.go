package wavinfo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, name string, entries []Entry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buildWAV(t, entries), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIInspectAndGet(t *testing.T) {
	path := writeFixture(t, "Artist - Title.wav", []Entry{
		{ID: "INAM", Text: "embedded"},
		{ID: "XUN1", payload: []byte("bespoke\x00")},
	})
	var out bytes.Buffer
	if err := RunCLI(CLIOptions{
		Args:   []string{"inspect", "-json", path},
		Stdout: &out,
	}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, `"id": "XUN1"`) || !strings.Contains(text, `"known": false`) {
		t.Fatalf("inspect json lost unknown field: %s", text)
	}

	out.Reset()
	if err := RunCLI(CLIOptions{
		Args:   []string{"get", path, "title"},
		Stdout: &out,
	}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "embedded" {
		t.Fatalf("get = %q", out.String())
	}
}

func TestCLISetPreservesDataAndRewrites(t *testing.T) {
	path := writeFixture(t, "song.wav", []Entry{{ID: "INAM", Text: "old"}})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := RunCLI(CLIOptions{
		Args:   []string{"set", path, "IART", "new artist"},
		Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeChunks, _ := ParseRIFF(bytes.NewReader(before))
	afterChunks, _ := ParseRIFF(bytes.NewReader(after))
	for i, b := range beforeChunks.chunks {
		if b.ID == "LIST" {
			continue
		}
		if !bytes.Equal(b.raw, afterChunks.chunks[i].raw) {
			t.Fatalf("chunk %s changed during CLI rewrite", b.ID)
		}
	}

	// The new field is retrievable through the same CLI surface.
	var out bytes.Buffer
	if err := RunCLI(CLIOptions{
		Args:   []string{"get", path, "artist"},
		Stdout: &out,
	}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "new artist" {
		t.Fatalf("artist = %q", out.String())
	}
}

func TestCLIApplyReportsConflictsAndSkips(t *testing.T) {
	path := writeFixture(t, "File Name.wav", []Entry{{ID: "INAM", Text: "tagged"}})
	var out bytes.Buffer
	err := RunCLI(CLIOptions{
		Args:       []string{"apply", path},
		Stdin:      strings.NewReader(`{"title":"project","unmapped-key":"v"}`),
		Stdout:     &out,
		Preference: PreferEmbedded,
	})
	if err != nil {
		t.Fatal(err)
	}
	report := out.String()
	if !strings.Contains(report, "conflict: INAM") ||
		!strings.Contains(report, "winner=embedded") ||
		!strings.Contains(report, "skipped (unresolved): unmapped-key") {
		t.Fatalf("report missing details: %s", report)
	}

	// Embedded value survives under the default preference.
	var got bytes.Buffer
	if err := RunCLI(CLIOptions{
		Args:   []string{"get", path, "INAM"},
		Stdout: &got,
	}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.String()) != "tagged" {
		t.Fatalf("embedded value overwritten: %q", got.String())
	}
}

func TestCLIErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		args []string
		code int
	}{
		{"no args", nil, 2},
		{"unknown command", []string{"frobnicate"}, 2},
		{"missing file", []string{"inspect"}, 2},
		{"unresolvable field", nil, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := tc.args
			if tc.name == "unresolvable field" {
				path := writeFixture(t, "ok.wav", nil)
				args = []string{"get", path, "not-a-valid-field"}
			}
			err := RunCLI(CLIOptions{Args: args, Stdout: &bytes.Buffer{}})
			cliErr, ok := err.(*CLIError)
			if !ok || cliErr.Code != tc.code {
				t.Fatalf("err = %v want code %d", err, tc.code)
			}
		})
	}
}

func TestCLIFieldsListsCanonicalOrder(t *testing.T) {
	var out bytes.Buffer
	if err := RunCLI(CLIOptions{
		Args:   []string{"fields"},
		Stdout: &out,
	}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != len(canonicalOrder) || lines[0] != "IARL\tArchival Location" {
		t.Fatalf("fields output = %v", lines)
	}
}

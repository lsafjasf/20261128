package releaseaudit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func completePackage() *Package {
	return &Package{
		SchemaVersion: SchemaVersion,
		Title:         "Example Feature",
		Versions: []Version{
			{
				ID:     "theatrical",
				Label:  "影院",
				Window: Window{Channel: "theatrical", Start: "2026-12-01", End: "2027-02-28"},
				Video: Video{
					Path: "theatrical/feature_theatrical.mxf", AspectRatio: "2.39:1",
					Width: 4096, Height: 1716,
				},
				Audio: Audio{
					Path:   "theatrical/feature_theatrical_51.wav",
					Layout: "5.1", Channels: 6, Language: "en",
				},
				Subtitles: []Subtitle{
					{Path: "theatrical/feature_theatrical_en.srt", Language: "en", Kind: "full"},
					{Path: "theatrical/feature_theatrical_zh.srt", Language: "zh-Hans", Kind: "sdh"},
				},
			},
			{
				ID:     "streaming",
				Label:  "流媒体",
				Window: Window{Channel: "streaming", Start: "2027-03-01", End: "2029-02-28"},
				Video: Video{
					Path: "streaming/feature_streaming.mxf", AspectRatio: "16:9",
					Width: 3840, Height: 2160,
				},
				Audio: Audio{
					Path:   "streaming/feature_streaming_51.wav",
					Layout: "5.1", Channels: 6, Language: "en",
				},
				Subtitles: []Subtitle{
					{Path: "streaming/feature_streaming_en.srt", Language: "en", Kind: "full"},
				},
			},
			{
				ID:     "mobile",
				Label:  "移动端",
				Window: Window{Channel: "mobile", Start: "2027-03-01", End: "2029-02-28"},
				Video: Video{
					Path: "mobile/feature_mobile.mp4", AspectRatio: "9:16",
					Width: 1080, Height: 1920,
				},
				Audio: Audio{
					Path:   "mobile/feature_mobile_stereo.m4a",
					Layout: "stereo", Channels: 2, Language: "en",
				},
				Subtitles: []Subtitle{
					{Path: "mobile/feature_mobile_en.vtt", Language: "en", Kind: "full"},
				},
			},
		},
		SharedManifest: []SharedManifest{{
			Path: "shared/release.json",
			Kind: "package",
			Targets: []ManifestTarget{
				{Version: "theatrical", References: []string{
					"theatrical/feature_theatrical.mxf",
					"theatrical/feature_theatrical_51.wav",
					"theatrical/feature_theatrical_en.srt",
				}},
				{Version: "streaming", References: []string{
					"streaming/feature_streaming.mxf",
					"streaming/feature_streaming_en.srt",
				}},
				{Version: "mobile", References: []string{
					"mobile/feature_mobile.mp4",
					"mobile/feature_mobile_en.vtt",
				}},
			},
		}},
	}
}

func TestCompletePackageMatrix(t *testing.T) {
	pkg := completePackage()
	report := Audit(pkg, Options{})
	if len(report.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", report.Issues)
	}
	if report.Summary.Versions != 3 || report.Summary.Errors != 0 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
	if got := report.Rows[0].AspectRatio; got != "2.39:1 (4096×1716)" {
		t.Fatalf("theatrical aspect = %q", got)
	}
	if got := report.Rows[1].AudioLayout; got != "5.1 (6ch)" {
		t.Fatalf("streaming audio = %q", got)
	}
	if got := report.Rows[2].Window; got != "2027-03-01 → 2029-02-28" {
		t.Fatalf("mobile window = %q", got)
	}
	if got := report.Rows[2].Status; got != "OK" {
		t.Fatalf("mobile status = %q", got)
	}
}

func TestDetectsMissingAndCrossReferences(t *testing.T) {
	pkg := completePackage()
	pkg.Versions[2].Video.Path = ""
	pkg.Versions[2].Subtitles = nil
	pkg.Versions[2].Window.End = ""
	pkg.Versions[1].Audio.Channels = 2
	pkg.Versions[0].Video.Width = 1920
	pkg.SharedManifest[0].Targets[2].References = []string{
		"theatrical/feature_theatrical.mxf",
	}

	report := Audit(pkg, Options{})
	for _, want := range []string{
		"missing_video_path",
		"missing_subtitles",
		"missing_window_end",
		"audio_layout_mismatch",
		"aspect_ratio_mismatch",
		"cross_version_reference",
		"manifest_target_not_versioned",
	} {
		if !hasCode(report, want) {
			t.Errorf("missing finding %s; got %+v", want, report.Issues)
		}
	}
	if report.Summary.CrossReferences == 0 || !report.HasErrors() {
		t.Fatalf("cross-reference summary not populated: %+v", report.Summary)
	}
}

func TestMissingRequiredVersionAndTarget(t *testing.T) {
	pkg := completePackage()
	pkg.Versions = pkg.Versions[:2]
	pkg.SharedManifest[0].Targets = pkg.SharedManifest[0].Targets[:1]

	report := Audit(pkg, Options{})
	if !hasCode(report, "missing_version") {
		t.Fatalf("missing missing_version: %+v", report.Issues)
	}
	if !hasCode(report, "missing_manifest_target") {
		t.Fatalf("missing target not reported: %+v", report.Issues)
	}
}

func TestSimilarFilenamesCannotCrossVersions(t *testing.T) {
	pkg := completePackage()
	pkg.Versions[1].Video.Path = "theatrical/feature_theatrical_streaming.mxf"
	pkg.SharedManifest[0].Targets[0].References = append(
		pkg.SharedManifest[0].Targets[0].References,
		"mobile/feature_mobile.mp4",
	)

	report := Audit(pkg, Options{})
	if !hasCode(report, "cross_version_reference") {
		t.Fatalf("cross-version reference not reported: %+v", report.Issues)
	}
	if !hasCode(report, "ambiguous_version_reference") && !hasCode(report, "cross_version_reference") {
		t.Fatalf("ambiguous filename not reported: %+v", report.Issues)
	}
}

func TestFilesystemChecks(t *testing.T) {
	root := t.TempDir()
	pkg := completePackage()
	pkg.SharedManifest = nil
	for _, version := range pkg.Versions {
		for _, path := range []string{version.Video.Path, version.Audio.Path} {
			writeFile(t, root, path)
		}
		for _, subtitle := range version.Subtitles {
			writeFile(t, root, subtitle.Path)
		}
	}
	report := Audit(pkg, Options{Root: root})
	if len(report.Issues) != 0 {
		t.Fatalf("unexpected issues with all files present: %+v", report.Issues)
	}

	if err := os.Remove(filepath.Join(root, "mobile/feature_mobile.mp4")); err != nil {
		t.Fatal(err)
	}
	report = Audit(pkg, Options{Root: root})
	if !hasCode(report, "missing_asset") {
		t.Fatalf("missing asset not reported: %+v", report.Issues)
	}
}

func TestSharedManifestJSONContentMustMatchTargets(t *testing.T) {
	root := t.TempDir()
	pkg := completePackage()
	for _, version := range pkg.Versions {
		writeFile(t, root, version.Video.Path)
		writeFile(t, root, version.Audio.Path)
		for _, subtitle := range version.Subtitles {
			writeFile(t, root, subtitle.Path)
		}
	}
	writeShared := func(targets []ManifestTarget) {
		t.Helper()
		data, err := json.MarshalIndent(sharedManifestFile{SchemaVersion: 1, Targets: targets}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, pkg.SharedManifest[0].Path)
		if err := os.WriteFile(filepath.Join(root, pkg.SharedManifest[0].Path), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeShared(pkg.SharedManifest[0].Targets)
	report := Audit(pkg, Options{Root: root})
	if len(report.Issues) != 0 {
		t.Fatalf("valid shared manifest failed: %+v", report.Issues)
	}

	wrong := append([]ManifestTarget(nil), pkg.SharedManifest[0].Targets...)
	wrong[0].References = []string{"mobile/feature_mobile.mp4"}
	writeShared(wrong)
	report = Audit(pkg, Options{Root: root})
	if !hasCode(report, "manifest_content_mismatch") {
		t.Fatalf("shared manifest content mismatch not reported: %+v", report.Issues)
	}
}

func TestLoadRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	if _, err := Load(strings.NewReader(`{"schemaVersion":1,"versions":[],"oops":true}`)); err == nil {
		t.Fatal("unknown field was accepted")
	}
	if _, err := Load(strings.NewReader(`{"schemaVersion":1,"versions":[]} {}`)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
}

func hasCode(report Report, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func writeFile(t *testing.T, root, path string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

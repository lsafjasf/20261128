package releaseaudit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Severity is the impact of a finding.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is one deterministic validation finding.
type Issue struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Version  string   `json:"version,omitempty"`
	Path     string   `json:"path,omitempty"`
	Message  string   `json:"message"`
}

// Row is one version in the distribution matrix.
type Row struct {
	Version     string   `json:"version"`
	Kind        Kind     `json:"kind,omitempty"`
	AspectRatio string   `json:"aspectRatio"`
	AudioLayout string   `json:"audioLayout"`
	Subtitles   string   `json:"subtitles"`
	Window      string   `json:"window"`
	Manifests   []string `json:"manifests,omitempty"`
	Status      string   `json:"status"`
}

// Summary counts the report at a glance.
type Summary struct {
	Versions        int `json:"versions"`
	Errors          int `json:"errors"`
	Warnings        int `json:"warnings"`
	Missing         int `json:"missing"`
	CrossReferences int `json:"crossReferences"`
}

// Report is both the machine-readable audit result and the matrix source.
type Report struct {
	Title   string  `json:"title,omitempty"`
	Rows    []Row   `json:"rows"`
	Issues  []Issue `json:"issues"`
	Summary Summary `json:"summary"`
}

// Options controls filesystem checks and the required version set.
type Options struct {
	// Root is the package directory used to resolve asset and manifest paths.
	// When empty, filesystem existence checks are skipped.
	Root string
	// RequiredKinds defaults to DefaultKinds.
	RequiredKinds []Kind
}

type auditor struct {
	pkg      *Package
	root     string
	required []Kind
	report   *Report

	versions  map[string]*Version
	kindOf    map[string]Kind
	assets    map[string][]string
	manifests map[string][]string
	rowIndex  map[string]int
}

// Audit validates the package and returns the complete matrix plus all
// findings. The package is not modified.
func Audit(pkg *Package, opts Options) Report {
	a := &auditor{
		pkg:       pkg,
		root:      strings.TrimSpace(opts.Root),
		required:  append([]Kind(nil), opts.RequiredKinds...),
		report:    &Report{Rows: []Row{}, Issues: []Issue{}},
		versions:  map[string]*Version{},
		kindOf:    map[string]Kind{},
		assets:    map[string][]string{},
		manifests: map[string][]string{},
		rowIndex:  map[string]int{},
	}
	if len(a.required) == 0 && pkg != nil {
		a.required = append([]Kind(nil), pkg.RequiredKinds...)
	}
	if len(a.required) == 0 {
		a.required = DefaultKinds
	}
	if pkg == nil {
		a.add("", "", SeverityError, "nil_package", "release package is nil")
		a.finish()
		return *a.report
	}

	a.report.Title = pkg.Title
	a.indexVersions()
	a.checkRequiredKinds()
	for i := range pkg.Versions {
		a.checkVersion(&pkg.Versions[i])
	}
	for i := range pkg.Versions {
		v := &pkg.Versions[i]
		a.checkLocalManifests(versionName(*v), a.kindOf[versionName(*v)], v)
	}
	a.checkSharedManifests()
	a.checkFiles()
	a.checkSharedManifestContent()
	a.finish()
	return *a.report
}

func (a *auditor) indexVersions() {
	for i := range a.pkg.Versions {
		v := &a.pkg.Versions[i]
		name := versionName(*v)
		a.report.Rows = append(a.report.Rows, Row{Version: name, Status: "OK"})
		if _, exists := a.rowIndex[name]; !exists {
			a.rowIndex[name] = len(a.report.Rows) - 1
		}
		if _, exists := a.versions[name]; exists {
			a.add(name, "", SeverityError, "duplicate_version_id",
				fmt.Sprintf("version id %q is declared more than once", name))
			continue
		}
		a.versions[name] = v
		if kind, ok := v.kind(); ok {
			a.kindOf[name] = kind
		}
	}
}

func (a *auditor) checkRequiredKinds() {
	byKind := map[Kind][]string{}
	for name, kind := range a.kindOf {
		byKind[kind] = append(byKind[kind], name)
	}
	for _, kind := range a.required {
		names := byKind[kind]
		switch len(names) {
		case 0:
			a.add("", "", SeverityError, "missing_version",
				fmt.Sprintf("required %s version is missing", kind))
		case 1:
			// Exactly one version for this distribution channel.
		default:
			sort.Strings(names)
			a.add("", "", SeverityError, "duplicate_version",
				fmt.Sprintf("multiple %s versions: %s", kind, strings.Join(names, ", ")))
			for _, name := range names {
				a.rowIssue(name, "", "duplicate_version",
					fmt.Sprintf("more than one %s version is declared", kind))
			}
		}
	}
	for i := range a.pkg.Versions {
		name := versionName(a.pkg.Versions[i])
		if _, ok := a.kindOf[name]; !ok {
			a.rowIssue(name, "", "unknown_version", "cannot classify version as theatrical, streaming, or mobile")
		}
	}
}

func (a *auditor) checkVersion(v *Version) {
	name := versionName(*v)
	kind := a.kindOf[name]
	a.checkWindow(name, kind, v)
	a.checkVideo(name, kind, v)
	a.checkAudio(name, kind, v)
	a.checkSubtitles(name, kind, v)
}

func (a *auditor) add(version, path string, severity Severity, code, message string) {
	a.report.Issues = append(a.report.Issues, Issue{
		Code: code, Severity: severity, Version: version, Path: path, Message: message,
	})
}

func (a *auditor) rowIssue(version, path, code, message string) {
	a.add(version, path, SeverityError, code, message)
}

func (a *auditor) checkPathOwnership(version string, kind Kind, path, usage string) {
	if path == "" {
		return
	}
	kinds := pathKinds(path)
	switch len(kinds) {
	case 0:
		return
	case 1:
		if kind != "" && kinds[0] != kind {
			a.add(version, path, SeverityError, "cross_version_reference",
				fmt.Sprintf("%s path names the %s version but belongs to the %s version", usage, kinds[0], kind))
		}
	default:
		a.add(version, path, SeverityError, "ambiguous_version_reference",
			fmt.Sprintf("%s path names multiple versions: %s", usage, joinKinds(kinds)))
	}
}

func (a *auditor) registerAsset(version string, kind Kind, path, usage string) {
	if path == "" || isRemote(path) {
		return
	}
	a.checkPathOwnership(version, kind, path, usage)
	key := pathKey(path)
	for _, owner := range a.assets[key] {
		if owner != version {
			a.add(version, path, SeverityError, "asset_shared_across_versions",
				fmt.Sprintf("%s is also declared by the %s version", usage, owner))
			a.add(owner, path, SeverityError, "asset_shared_across_versions",
				fmt.Sprintf("%s is also declared by the %s version", usage, version))
			return
		}
	}
	a.assets[key] = append(a.assets[key], version)
}

func (a *auditor) registerManifest(version, path string, local bool) {
	if path == "" || isRemote(path) {
		return
	}
	key := pathKey(path)
	for _, owner := range a.manifests[key] {
		if owner != version {
			code := "implicitly_shared_manifest"
			if !local {
				code = "duplicate_manifest_path"
			}
			msg := fmt.Sprintf("manifest %q is declared by both %s and %s; shared manifests must use sharedManifests", path, owner, version)
			a.add(version, path, SeverityError, code, msg)
			a.add(owner, path, SeverityError, code, msg)
			return
		}
	}
	a.manifests[key] = append(a.manifests[key], version)
}

func (a *auditor) finish() {
	for i := range a.report.Rows {
		a.report.Rows[i].Status = "OK"
	}
	for _, issue := range a.report.Issues {
		if issue.Severity == SeverityError {
			a.report.Summary.Errors++
		} else {
			a.report.Summary.Warnings++
		}
		if strings.HasPrefix(issue.Code, "missing_") {
			a.report.Summary.Missing++
		}
		if isCrossReferenceCode(issue.Code) {
			a.report.Summary.CrossReferences++
		}
		if idx, ok := a.rowIndex[issue.Version]; ok {
			if issue.Severity == SeverityError {
				a.report.Rows[idx].Status = "ERROR"
			} else if a.report.Rows[idx].Status != "ERROR" {
				a.report.Rows[idx].Status = "WARN"
			}
		}
	}
	a.report.Summary.Versions = len(a.report.Rows)
}

func isCrossReferenceCode(code string) bool {
	switch code {
	case "cross_version_reference",
		"ambiguous_version_reference",
		"asset_shared_across_versions",
		"implicitly_shared_manifest",
		"manifest_path_conflict",
		"manifest_path_version_mismatch",
		"manifest_content_mismatch",
		"manifest_target_not_versioned",
		"manifest_not_versioned",
		"window_version_mismatch":
		return true
	}
	return false
}

func pathKey(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "./")
}

func isRemote(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func joinKinds(kinds []Kind) string {
	parts := make([]string, len(kinds))
	for i, kind := range kinds {
		parts[i] = string(kind)
	}
	return strings.Join(parts, ", ")
}

func (a *auditor) checkFiles() {
	if a.root == "" {
		return
	}
	root, err := filepath.Abs(a.root)
	if err != nil {
		a.add("", a.root, SeverityError, "invalid_root", err.Error())
		return
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		a.add("", a.root, SeverityError, "invalid_root", "package root does not exist or is not a directory")
		return
	}

	seen := map[string]bool{}
	for i := range a.pkg.Versions {
		v := &a.pkg.Versions[i]
		name := versionName(*v)
		paths := []struct{ path, usage string }{
			{v.Video.Path, "video"},
			{v.Audio.Path, "audio"},
		}
		for _, sub := range v.Subtitles {
			paths = append(paths, struct{ path, usage string }{sub.Path, "subtitle"})
		}
		for _, manifest := range v.Manifests {
			paths = append(paths, struct{ path, usage string }{manifest.Path, "manifest"})
			for _, ref := range manifest.References {
				paths = append(paths, struct{ path, usage string }{ref, "manifest reference"})
			}
		}
		for _, item := range paths {
			a.checkFile(root, name, item.path, item.usage, seen)
		}
	}
	for _, shared := range a.pkg.SharedManifest {
		a.checkFile(root, "", shared.Path, "shared manifest", seen)
		for _, target := range shared.Targets {
			for _, ref := range target.References {
				a.checkFile(root, target.Version, ref, "shared manifest reference", seen)
			}
		}
	}
}

func (a *auditor) checkFile(root, version, path, usage string, seen map[string]bool) {
	if path == "" || isRemote(path) {
		return
	}
	key := pathKey(path)
	if seen[key] {
		return
	}
	seen[key] = true

	full, err := secureJoin(root, path)
	if err != nil {
		a.add(version, path, SeverityError, "path_outside_root",
			fmt.Sprintf("%s path escapes the package root: %v", usage, err))
		return
	}
	info, err := os.Stat(full)
	if err != nil {
		a.add(version, path, SeverityError, "missing_asset",
			fmt.Sprintf("%s file is missing: %s", usage, path))
		return
	}
	if !info.Mode().IsRegular() {
		a.add(version, path, SeverityError, "invalid_asset",
			fmt.Sprintf("%s path is not a regular file: %s", usage, path))
	}
}

func secureJoin(root, path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path leaves the package root")
	}
	full := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path leaves the package root")
	}
	return full, nil
}

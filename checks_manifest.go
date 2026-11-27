package releaseaudit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (a *auditor) checkLocalManifests(name string, kind Kind, v *Version) {
	seenManifests := map[string]bool{}
	for _, manifest := range v.Manifests {
		path := strings.TrimSpace(manifest.Path)
		if path == "" {
			a.rowIssue(name, "", "missing_manifest_path", "version manifest path is missing")
			continue
		}
		key := pathKey(path)
		if seenManifests[key] {
			a.add(name, path, SeverityError, "duplicate_manifest_path",
				fmt.Sprintf("version manifest %q is declared more than once", path))
			continue
		}
		seenManifests[key] = true
		a.checkPathOwnership(name, kind, path, "manifest")
		a.registerManifest(name, path, true)
		a.addRowManifest(name, path)

		if len(manifest.References) == 0 {
			a.rowIssue(name, path, "missing_manifest_references",
				"version manifest has no references")
			continue
		}
		a.checkReferenceSet(name, kind, path, manifest.References,
			"version manifest", "manifest_not_versioned")
	}
}

func (a *auditor) checkSharedManifests() {
	sharedSeen := map[string]bool{}
	for _, shared := range a.pkg.SharedManifest {
		path := strings.TrimSpace(shared.Path)
		if path == "" {
			a.rowIssue("", "", "missing_manifest_path", "shared manifest path is missing")
			continue
		}
		key := pathKey(path)
		if sharedSeen[key] {
			a.add("", path, SeverityError, "duplicate_manifest_path",
				fmt.Sprintf("shared manifest %q is declared more than once", path))
		}
		sharedSeen[key] = true
		for _, owner := range a.manifests[key] {
			a.add(owner, path, SeverityError, "manifest_path_conflict",
				fmt.Sprintf("manifest %q is declared both locally and as shared", path))
		}
		if kinds := pathKinds(path); len(kinds) > 0 {
			a.add("", path, SeverityError, "manifest_path_version_mismatch",
				fmt.Sprintf("shared manifest path names %s and cannot be shared safely", joinKinds(kinds)))
		}
		if len(shared.Targets) == 0 {
			a.add("", path, SeverityError, "missing_manifest_targets",
				"shared manifest has no version targets")
			continue
		}

		targeted := map[string]bool{}
		for _, target := range shared.Targets {
			name, kind, ok, ambiguous := a.resolveVersion(target.Version)
			switch {
			case ambiguous:
				a.add("", path, SeverityError, "ambiguous_manifest_target",
					fmt.Sprintf("shared manifest target %q matches more than one version", target.Version))
				continue
			case !ok:
				a.add("", path, SeverityError, "manifest_unknown_target",
					fmt.Sprintf("shared manifest target %q is not a declared version", target.Version))
				continue
			}
			if targeted[name] {
				a.add(name, path, SeverityError, "duplicate_manifest_target",
					fmt.Sprintf("version %s appears more than once in the shared manifest", name))
				continue
			}
			targeted[name] = true
			a.addRowManifest(name, path)
			if len(target.References) == 0 {
				a.rowIssue(name, path, "missing_manifest_references",
					"shared manifest target has no references")
				continue
			}
			a.checkReferenceSet(name, kind, path, target.References,
				"shared manifest target", "manifest_target_not_versioned")
		}

		for i := range a.pkg.Versions {
			name := versionName(a.pkg.Versions[i])
			if !targeted[name] {
				a.add(name, path, SeverityError, "missing_manifest_target",
					fmt.Sprintf("shared manifest has no target for version %s", name))
			}
		}
	}
}

func (a *auditor) resolveVersion(ref string) (name string, kind Kind, ok, ambiguous bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", false, false
	}
	if _, exists := a.versions[ref]; exists {
		return ref, a.kindOf[ref], true, false
	}
	resolvedKind, classified := ClassifyKind(ref)
	if !classified {
		for i := range a.pkg.Versions {
			v := &a.pkg.Versions[i]
			if strings.EqualFold(strings.TrimSpace(v.Label), ref) {
				n := versionName(*v)
				return n, a.kindOf[n], true, false
			}
		}
		return "", "", false, false
	}
	var matches []string
	for i := range a.pkg.Versions {
		n := versionName(a.pkg.Versions[i])
		if a.kindOf[n] == resolvedKind {
			matches = append(matches, n)
		}
	}
	if len(matches) == 1 {
		return matches[0], resolvedKind, true, false
	}
	if len(matches) > 1 {
		return "", resolvedKind, false, true
	}
	return "", resolvedKind, false, false
}

func (a *auditor) checkReferenceSet(version string, kind Kind, manifestPath string, refs []string, usage, unconfirmedCode string) {
	seen := map[string]bool{}
	confirmed := false
	hasReference := false
	for _, raw := range refs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			a.add(version, manifestPath, SeverityError, "missing_manifest_reference",
				usage+" contains an empty reference")
			continue
		}
		hasReference = true
		key := pathKey(ref)
		if seen[key] {
			a.add(version, ref, SeverityError, "duplicate_manifest_reference",
				fmt.Sprintf("%s references %q more than once", usage, ref))
			continue
		}
		seen[key] = true
		if a.checkManifestReference(version, kind, ref, usage) {
			confirmed = true
		}
	}
	if hasReference && !confirmed {
		a.add(version, manifestPath, SeverityError, unconfirmedCode,
			fmt.Sprintf("%s does not establish a reference to the %s version", usage, version))
	}
}

func (a *auditor) checkManifestReference(version string, kind Kind, ref, usage string) bool {
	confirmed := false
	kinds := pathKinds(ref)
	switch len(kinds) {
	case 1:
		if kind != "" && kinds[0] != kind {
			a.add(version, ref, SeverityError, "cross_version_reference",
				fmt.Sprintf("%s names the %s version but belongs to the %s version", usage, kinds[0], kind))
			return false
		}
		confirmed = kind != "" && kinds[0] == kind
	case 2:
		fallthrough
	default:
		if len(kinds) > 1 {
			a.add(version, ref, SeverityError, "ambiguous_version_reference",
				fmt.Sprintf("%s names multiple versions: %s", usage, joinKinds(kinds)))
			return false
		}
	}

	key := pathKey(ref)
	crossed := false
	for _, owner := range a.assets[key] {
		if owner != version {
			a.add(version, ref, SeverityError, "cross_version_reference",
				fmt.Sprintf("%s points to an asset owned by the %s version", usage, owner))
			crossed = true
			continue
		}
		confirmed = true
	}
	return confirmed && !crossed
}

func (a *auditor) addRowManifest(version, path string) {
	row := a.row(version)
	if row == nil || path == "" {
		return
	}
	for _, existing := range row.Manifests {
		if existing == path {
			return
		}
	}
	row.Manifests = append(row.Manifests, path)
}

type sharedManifestFile struct {
	SchemaVersion int              `json:"schemaVersion,omitempty"`
	Targets       []ManifestTarget `json:"targets"`
}

func (a *auditor) checkSharedManifestContent() {
	if a.root == "" {
		return
	}
	root, err := filepath.Abs(a.root)
	if err != nil {
		return
	}
	for _, shared := range a.pkg.SharedManifest {
		path := strings.TrimSpace(shared.Path)
		if path == "" || isRemote(path) || strings.ToLower(filepath.Ext(path)) != ".json" {
			continue
		}
		full, err := secureJoin(root, path)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			a.add("", path, SeverityError, "invalid_manifest",
				fmt.Sprintf("cannot read shared manifest: %v", err))
			continue
		}
		var content sharedManifestFile
		if err := json.Unmarshal(data, &content); err != nil {
			a.add("", path, SeverityError, "invalid_manifest",
				fmt.Sprintf("cannot parse shared manifest JSON: %v", err))
			continue
		}
		declared := a.canonicalTargets(shared.Targets)
		actual := a.canonicalTargets(content.Targets)
		if diffs := diffTargets(declared, actual); len(diffs) > 0 {
			a.add("", path, SeverityError, "manifest_content_mismatch",
				"shared manifest content differs from declared targets: "+strings.Join(diffs, "; "))
		}
	}
}

func (a *auditor) canonicalTargets(targets []ManifestTarget) map[string][]string {
	result := map[string][]string{}
	for _, target := range targets {
		key := strings.TrimSpace(target.Version)
		if name, _, ok, ambiguous := a.resolveVersion(key); ok && !ambiguous {
			key = name
		} else {
			key = "?" + key
		}
		for _, ref := range target.References {
			result[key] = append(result[key], pathKey(strings.TrimSpace(ref)))
		}
		sort.Strings(result[key])
	}
	return result
}

func diffTargets(declared, actual map[string][]string) []string {
	keys := map[string]bool{}
	for key := range declared {
		keys[key] = true
	}
	for key := range actual {
		keys[key] = true
	}
	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)

	var diffs []string
	for _, key := range sorted {
		if !equalStrings(declared[key], actual[key]) {
			diffs = append(diffs, fmt.Sprintf("%s declared=%v actual=%v",
				strings.TrimPrefix(key, "?"), declared[key], actual[key]))
		}
	}
	return diffs
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

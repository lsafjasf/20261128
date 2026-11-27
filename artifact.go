package renderpipe

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	ManifestFileName   = "manifest.json"
	ManifestSchema     = "renderpipe/artifact/v1"
	FrameDirectoryName = "frames"
)

var (
	// ErrArtifactConflict means an artifact path already exists. Render never
	// deletes or replaces it automatically.
	ErrArtifactConflict = errors.New("render artifact already exists; refusing to overwrite")
	// ErrInvalidArtifact means an artifact's manifest or content is inconsistent.
	ErrInvalidArtifact = errors.New("invalid render artifact")
)

// ArtifactFile records the stable, artifact-relative name and content hash of
// one output file.
type ArtifactFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// ArtifactManifest is both the success marker and the content index for a
// render. It intentionally contains no timestamps or host-local paths.
type ArtifactManifest struct {
	Schema       string         `json:"schema"`
	TaskID       string         `json:"task_id"`
	SourceHash   string         `json:"source_sha256"`
	ConfigHash   string         `json:"config_sha256"`
	Renderer     string         `json:"renderer"`
	FPS          int            `json:"fps"`
	FramePattern string         `json:"frame_pattern"`
	FrameSetHash string         `json:"frame_set_sha256"`
	Frames       []ArtifactFile `json:"frames"`
	Media        ArtifactFile   `json:"media"`
}

// RenderResult describes an artifact that was either just published or safely
// reused after verifying its full manifest.
type RenderResult struct {
	TaskID       string   `json:"task_id"`
	SourceHash   string   `json:"source_sha256"`
	ConfigHash   string   `json:"config_sha256"`
	Directory    string   `json:"directory"`
	MediaPath    string   `json:"media_path"`
	MediaHash    string   `json:"media_sha256"`
	FrameSetHash string   `json:"frame_set_sha256"`
	Frames       []string `json:"frames"`
	Reused       bool     `json:"reused"`
}

func validateManifest(manifest ArtifactManifest) error {
	if manifest.Schema != ManifestSchema {
		return fmt.Errorf("unsupported manifest schema %q", manifest.Schema)
	}
	if !strings.HasPrefix(manifest.TaskID, TaskPrefix) || len(manifest.TaskID) != len(TaskPrefix)+64 {
		return fmt.Errorf("invalid task id %q", manifest.TaskID)
	}
	if !validDigest(manifest.SourceHash) || !validDigest(manifest.ConfigHash) {
		return fmt.Errorf("manifest is missing source or config hash")
	}
	if !validDigest(manifest.FrameSetHash) {
		return fmt.Errorf("manifest has an invalid frame set hash")
	}
	if manifest.FPS < 1 || manifest.FPS > 1000 || manifest.FramePattern == "" || manifest.Media.Path == "" {
		return fmt.Errorf("manifest is missing render metadata")
	}
	if manifest.Renderer != rendererVersion {
		return fmt.Errorf("unsupported renderer %q", manifest.Renderer)
	}
	if len(manifest.Frames) == 0 {
		return fmt.Errorf("manifest contains no frames")
	}
	expectedPattern := framePatternFor(len(manifest.Frames))
	if manifest.FramePattern != expectedPattern {
		return fmt.Errorf("frame pattern %q does not match %d frames", manifest.FramePattern, len(manifest.Frames))
	}
	for i, frame := range manifest.Frames {
		expected := FrameDirectoryName + "/" + frameName(i+1, len(manifest.Frames))
		if frame.Path != expected {
			return fmt.Errorf("frame %d is named %q, expected %q", i+1, frame.Path, expected)
		}
		if !validDigest(frame.SHA256) || frame.Size < 0 {
			return fmt.Errorf("frame %d has invalid content metadata", i+1)
		}
	}
	if manifest.Media.Path != mediaFileName {
		return fmt.Errorf("unexpected media path %q", manifest.Media.Path)
	}
	if !validDigest(manifest.Media.SHA256) || manifest.Media.Size < 0 {
		return fmt.Errorf("media has invalid content metadata")
	}
	if got := frameSetHash(manifest.Frames); got != manifest.FrameSetHash {
		return fmt.Errorf("frame set hash mismatch")
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, b := range []byte(value) {
		if !((b >= '0' && b <= '9') || (b >= 'a' && b <= 'f')) {
			return false
		}
	}
	return true
}

// VerifyArtifact checks that dir contains a complete and self-consistent
// successful artifact. It does not modify the directory.
func VerifyArtifact(dir string) (ArtifactManifest, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ArtifactManifest{}, fmt.Errorf("%w: resolve directory: %v", ErrInvalidArtifact, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ManifestFileName))
	if err != nil {
		return ArtifactManifest{}, fmt.Errorf("%w: read manifest: %v", ErrInvalidArtifact, err)
	}
	var manifest ArtifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ArtifactManifest{}, fmt.Errorf("%w: parse manifest: %v", ErrInvalidArtifact, err)
	}
	if err := validateManifest(manifest); err != nil {
		return ArtifactManifest{}, fmt.Errorf("%w: %v", ErrInvalidArtifact, err)
	}
	if filepath.Base(dir) != manifest.TaskID {
		return ArtifactManifest{}, fmt.Errorf("%w: directory name does not match task id", ErrInvalidArtifact)
	}
	for _, file := range append(append([]ArtifactFile(nil), manifest.Frames...), manifest.Media) {
		if err := verifyArtifactFile(dir, file); err != nil {
			return ArtifactManifest{}, fmt.Errorf("%w: %v", ErrInvalidArtifact, err)
		}
	}
	return manifest, nil
}

func verifyArtifactFile(root string, file ArtifactFile) error {
	clean, err := safeArtifactPath(file.Path)
	if err != nil {
		return err
	}
	abs := filepath.Join(root, filepath.FromSlash(clean))
	info, err := os.Lstat(abs)
	if err != nil {
		return fmt.Errorf("stat %q: %w", file.Path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", file.Path)
	}
	digest, size, err := hashFile(abs)
	if err != nil {
		return err
	}
	if size != file.Size {
		return fmt.Errorf("%q size mismatch", file.Path)
	}
	if digest != file.SHA256 {
		return fmt.Errorf("%q hash mismatch", file.Path)
	}
	return nil
}

func safeArtifactPath(value string) (string, error) {
	if value == "" || strings.Contains(value, `\`) {
		return "", fmt.Errorf("invalid artifact path %q", value)
	}
	clean := path.Clean(value)
	if path.IsAbs(value) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("artifact path %q escapes its directory", value)
	}
	return clean, nil
}

func frameSetHash(frames []ArtifactFile) string {
	fields := make([]string, 0, len(frames)*3+1)
	for _, frame := range frames {
		fields = append(fields, frame.Path, frame.SHA256, fmt.Sprint(frame.Size))
	}
	return joinHash("renderpipe/frame-set/v1", fields...)
}

func artifactResult(dir string, manifest ArtifactManifest, reused bool) RenderResult {
	frames := make([]string, len(manifest.Frames))
	for i, frame := range manifest.Frames {
		frames[i] = filepath.Join(dir, filepath.FromSlash(frame.Path))
	}
	return RenderResult{
		TaskID:       manifest.TaskID,
		SourceHash:   manifest.SourceHash,
		ConfigHash:   manifest.ConfigHash,
		Directory:    dir,
		MediaPath:    filepath.Join(dir, filepath.FromSlash(manifest.Media.Path)),
		MediaHash:    manifest.Media.SHA256,
		FrameSetHash: manifest.FrameSetHash,
		Frames:       frames,
		Reused:       reused,
	}
}

func inspectExisting(plan TaskPlan) (RenderResult, bool, error) {
	info, err := os.Lstat(plan.Directory)
	if err != nil {
		if os.IsNotExist(err) {
			return RenderResult{}, false, nil
		}
		return RenderResult{}, false, fmt.Errorf("inspect artifact path: %w", err)
	}
	if !info.IsDir() {
		return RenderResult{}, false, fmt.Errorf("%w: %s", ErrArtifactConflict, plan.Directory)
	}
	manifest, err := VerifyArtifact(plan.Directory)
	if err != nil {
		return RenderResult{}, false, fmt.Errorf("%w: existing path is not reusable: %v", ErrArtifactConflict, err)
	}
	if manifest.TaskID != plan.TaskID || manifest.SourceHash != plan.SourceHash ||
		manifest.ConfigHash != plan.ConfigHash {
		return RenderResult{}, false, fmt.Errorf("%w: existing manifest does not match task", ErrArtifactConflict)
	}
	return artifactResult(plan.Directory, manifest, true), true, nil
}

func writeManifest(dir string, manifest ArtifactManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(dir, ManifestFileName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return syncPath(path)
}

func syncPath(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func ensureUnchangedExisting(plan TaskPlan) (RenderResult, bool, error) {
	result, ok, err := inspectExisting(plan)
	if err != nil {
		return RenderResult{}, false, err
	}
	if ok {
		return result, true, nil
	}
	return RenderResult{}, false, nil
}

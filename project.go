package renderpipe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	ProjectFileName = "project.json"

	DefaultFPS      = 10
	DefaultFrameDir = "frames"

	rendererVersion = "renderpipe/gif/v1"
	framePattern    = "frame-%06d.png"
	mediaFileName   = "animation.gif"
)

// Project is the checked-in rendering specification. If Frames is empty,
// supported image files below FrameDir are discovered and naturally sorted.
// Otherwise, Frames is interpreted as an explicit, ordered list.
type Project struct {
	Version  int      `json:"version,omitempty"`
	FPS      int      `json:"fps,omitempty"`
	FrameDir string   `json:"frame_dir,omitempty"`
	Frames   []string `json:"frames,omitempty"`
}

type projectFile struct {
	Version  int      `json:"version"`
	FPS      *int     `json:"fps"`
	FrameDir *string  `json:"frame_dir"`
	Frames   []string `json:"frames"`
}

type frameInput struct {
	Path string
	Abs  string
}

func loadProject(root string) (Project, error) {
	project := Project{
		Version:  1,
		FPS:      DefaultFPS,
		FrameDir: DefaultFrameDir,
	}
	data, err := os.ReadFile(filepath.Join(root, ProjectFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return project, nil
		}
		return Project{}, fmt.Errorf("read %s: %w", ProjectFileName, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var raw projectFile
	if err := decoder.Decode(&raw); err != nil {
		return Project{}, fmt.Errorf("parse %s: %w", ProjectFileName, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Project{}, fmt.Errorf("parse %s: %w", ProjectFileName, err)
	}
	if raw.Version != 0 {
		project.Version = raw.Version
	}
	if raw.FPS != nil {
		project.FPS = *raw.FPS
	}
	if raw.FrameDir != nil {
		project.FrameDir = *raw.FrameDir
	}
	project.Frames = append([]string(nil), raw.Frames...)
	return project, validateProject(project)
}

func validateProject(project Project) error {
	if project.Version != 1 {
		return fmt.Errorf("unsupported project version %d", project.Version)
	}
	if project.FPS < 1 || project.FPS > 1000 {
		return fmt.Errorf("fps must be between 1 and 1000, got %d", project.FPS)
	}
	if _, err := safeRelativePath(project.FrameDir); err != nil {
		return fmt.Errorf("invalid frame_dir: %w", err)
	}
	seen := make(map[string]struct{}, len(project.Frames))
	for i, frame := range project.Frames {
		clean, err := safeRelativePath(frame)
		if err != nil {
			return fmt.Errorf("invalid frames[%d]: %w", i, err)
		}
		project.Frames[i] = slashPath(clean)
		if _, ok := seen[project.Frames[i]]; ok {
			return fmt.Errorf("duplicate frame %q", project.Frames[i])
		}
		seen[project.Frames[i]] = struct{}{}
	}
	return nil
}

func discoverFrames(root string, project Project) ([]frameInput, error) {
	if len(project.Frames) > 0 {
		frames := make([]frameInput, 0, len(project.Frames))
		for _, rel := range project.Frames {
			frames = append(frames, frameInput{
				Path: rel,
				Abs:  filepath.Join(root, filepath.FromSlash(rel)),
			})
		}
		return validateFrames(frames)
	}

	frameRoot := filepath.Join(root, filepath.FromSlash(project.FrameDir))
	info, err := os.Stat(frameRoot)
	if err != nil {
		return nil, fmt.Errorf("stat frame_dir %q: %w", project.FrameDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("frame_dir %q is not a directory", project.FrameDir)
	}

	var frames []frameInput
	err = filepath.WalkDir(frameRoot, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == frameRoot {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("frame %q must not be a symlink", current)
		}
		if !entry.Type().IsRegular() || !supportedImage(entry.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		rel = slashPath(rel)
		frames = append(frames, frameInput{Path: rel, Abs: current})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan frame_dir %q: %w", project.FrameDir, err)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no supported frames found below %q", project.FrameDir)
	}
	for i := range frames {
		frames[i].Path = slashPath(frames[i].Path)
	}
	sortFrames(frames)
	return frames, nil
}

func validateFrames(frames []frameInput) ([]frameInput, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("project has no frames")
	}
	for i, frame := range frames {
		info, err := os.Lstat(frame.Abs)
		if err != nil {
			return nil, fmt.Errorf("stat frame %q: %w", frame.Path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("frame %q is not a regular file", frame.Path)
		}
		if !supportedImage(frame.Path) {
			return nil, fmt.Errorf("frame %q has an unsupported image extension", frame.Path)
		}
		frames[i] = frame
	}
	return frames, nil
}

func supportedImage(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif":
		return true
	default:
		return false
	}
}

func safeRelativePath(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(value) || path.IsAbs(value) {
		return "", fmt.Errorf("path %q must be relative", value)
	}
	clean := path.Clean(filepath.ToSlash(value))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q escapes the project root", value)
	}
	return clean, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected data after JSON object")
		}
		return err
	}
	return nil
}

func sortFrames(frames []frameInput) {
	sort.Slice(frames, func(i, j int) bool {
		return naturalLess(frames[i].Path, frames[j].Path)
	})
}

func naturalLess(a, b string) bool {
	for i, j := 0, 0; i < len(a) && j < len(b); {
		if isDigit(a[i]) && isDigit(b[j]) {
			ai, aj := i, j
			for ai < len(a) && isDigit(a[ai]) {
				ai++
			}
			for aj < len(b) && isDigit(b[aj]) {
				aj++
			}
			araw, braw := a[i:ai], b[j:aj]
			atrim := strings.TrimLeft(araw, "0")
			btrim := strings.TrimLeft(braw, "0")
			if atrim == "" {
				atrim = "0"
			}
			if btrim == "" {
				btrim = "0"
			}
			if len(atrim) != len(btrim) {
				return len(atrim) < len(btrim)
			}
			if atrim != btrim {
				return atrim < btrim
			}
			if len(araw) != len(braw) {
				return len(araw) < len(braw)
			}
			if araw != braw {
				return araw < braw
			}
			i, j = ai, aj
			continue
		}
		if a[i] != b[j] {
			return a[i] < b[j]
		}
		i++
		j++
	}
	return len(a) < len(b)
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func frameName(index, total int) string {
	width := len(strconv.Itoa(total))
	if width < 6 {
		width = 6
	}
	return fmt.Sprintf("frame-%0*d.png", width, index)
}

func framePatternFor(total int) string {
	width := len(strconv.Itoa(total))
	if width < 6 {
		width = 6
	}
	return fmt.Sprintf("%s/frame-%%0%dd.png", FrameDirectoryName, width)
}

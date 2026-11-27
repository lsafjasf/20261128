package renderpipe

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	TaskPrefix   = "rt1-"
	configDomain = "renderpipe/config/v1"
	taskDomain   = "renderpipe/task/v1"
)

// TaskPlan is the complete, deterministic output contract for one render.
type TaskPlan struct {
	TaskID       string   `json:"task_id"`
	SourceHash   string   `json:"source_sha256"`
	ConfigHash   string   `json:"config_sha256"`
	Directory    string   `json:"artifact"`
	FPS          int      `json:"fps"`
	FramePattern string   `json:"frame_pattern"`
	MediaName    string   `json:"media_name"`
	Frames       []string `json:"frames"`
}

// PlanTask resolves a project without writing any files. The returned task ID
// and directory depend only on source content, frame order, and render options.
func PlanTask(projectRoot, outputRoot string) (TaskPlan, error) {
	projectRoot, outputRoot, err := resolveRoots(projectRoot, outputRoot)
	if err != nil {
		return TaskPlan{}, err
	}
	project, err := loadProject(projectRoot)
	if err != nil {
		return TaskPlan{}, err
	}
	frames, err := discoverFrames(projectRoot, project)
	if err != nil {
		return TaskPlan{}, err
	}

	ignores, err := outputIgnores(projectRoot, outputRoot, project)
	if err != nil {
		return TaskPlan{}, err
	}
	snapshot, err := SnapshotSource(projectRoot, SourceOptions{Ignore: ignores})
	if err != nil {
		return TaskPlan{}, err
	}

	framePaths := make([]string, len(frames))
	for i, frame := range frames {
		framePaths[i] = frame.Path
	}
	configHash := renderConfigHash(project.FPS, framePaths)
	taskID := TaskPrefix + joinHash(taskDomain, snapshot.Hash, configHash)
	return TaskPlan{
		TaskID:       taskID,
		SourceHash:   snapshot.Hash,
		ConfigHash:   configHash,
		Directory:    filepath.Join(outputRoot, taskID),
		FPS:          project.FPS,
		FramePattern: framePatternFor(len(frames)),
		MediaName:    mediaFileName,
		Frames:       framePaths,
	}, nil
}

func resolveRoots(projectRoot, outputRoot string) (string, string, error) {
	if projectRoot == "" {
		return "", "", fmt.Errorf("project root is required")
	}
	if outputRoot == "" {
		return "", "", fmt.Errorf("output root is required")
	}
	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve project root: %w", err)
	}
	outputRoot, err = filepath.Abs(outputRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve output root: %w", err)
	}
	info, err := os.Stat(projectRoot)
	if err != nil {
		return "", "", fmt.Errorf("stat project root: %w", err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("project root %q is not a directory", projectRoot)
	}
	if projectRoot == outputRoot {
		return "", "", fmt.Errorf("output root must differ from project root")
	}
	if info, err := os.Stat(outputRoot); err == nil && !info.IsDir() {
		return "", "", fmt.Errorf("output root %q is not a directory", outputRoot)
	} else if err != nil && !os.IsNotExist(err) {
		return "", "", fmt.Errorf("stat output root: %w", err)
	}
	return projectRoot, outputRoot, nil
}

func outputIgnores(projectRoot, outputRoot string, project Project) ([]string, error) {
	rel, err := filepath.Rel(projectRoot, outputRoot)
	if err != nil {
		return nil, fmt.Errorf("relate output root to project root: %w", err)
	}
	rel = slashPath(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return nil, nil
	}
	if rel == "." {
		return nil, fmt.Errorf("output root must not be the project root")
	}
	if len(project.Frames) == 0 {
		frameRoot, err := safeRelativePath(project.FrameDir)
		if err != nil {
			return nil, err
		}
		if rel == frameRoot || strings.HasPrefix(rel, frameRoot+"/") {
			return nil, fmt.Errorf("output root must not be inside frame_dir %q", frameRoot)
		}
	}
	return []string{rel}, nil
}

func renderConfigHash(fps int, frames []string) string {
	fields := []string{
		strconv.Itoa(fps),
		framePattern,
		mediaFileName,
		rendererVersion,
		strconv.Itoa(len(frames)),
	}
	fields = append(fields, frames...)
	return joinHash(configDomain, fields...)
}

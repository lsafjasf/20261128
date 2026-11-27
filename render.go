package renderpipe

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

var ErrSourceChanged = errors.New("project source changed while rendering")

// Render creates a deterministic artifact for projectRoot below outputRoot.
// If the same complete artifact already exists, it is verified and reused
// without modifying any of its files.
func Render(projectRoot, outputRoot string) (RenderResult, error) {
	plan, err := PlanTask(projectRoot, outputRoot)
	if err != nil {
		return RenderResult{}, err
	}
	if result, ok, err := ensureUnchangedExisting(plan); err != nil {
		return RenderResult{}, err
	} else if ok {
		return result, nil
	}

	outputRoot = filepath.Dir(plan.Directory)
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		return RenderResult{}, fmt.Errorf("create output root: %w", err)
	}
	staging, err := os.MkdirTemp(outputRoot, ".renderpipe-staging-")
	if err != nil {
		return RenderResult{}, fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	manifest, err := renderInto(staging, projectRoot, plan)
	if err != nil {
		return RenderResult{}, err
	}

	// The source may change while frames are being read. Recompute the full
	// source hash and discard this attempt if the task contract moved.
	current, err := PlanTask(projectRoot, outputRoot)
	if err != nil {
		return RenderResult{}, err
	}
	if current.TaskID != plan.TaskID || current.SourceHash != plan.SourceHash {
		return RenderResult{}, ErrSourceChanged
	}

	if result, ok, err := ensureUnchangedExisting(plan); err != nil {
		return RenderResult{}, err
	} else if ok {
		return result, nil
	}
	if err := os.Rename(staging, plan.Directory); err != nil {
		// A concurrent renderer may have published the same task first.
		if result, ok, inspectErr := ensureUnchangedExisting(plan); inspectErr == nil && ok {
			return result, nil
		}
		return RenderResult{}, fmt.Errorf("%w: publish artifact: %v", ErrArtifactConflict, err)
	}
	return artifactResult(plan.Directory, manifest, false), nil
}

func renderInto(staging, projectRoot string, plan TaskPlan) (ArtifactManifest, error) {
	frameDir := filepath.Join(staging, FrameDirectoryName)
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return ArtifactManifest{}, fmt.Errorf("create frame directory: %w", err)
	}

	anim := &gif.GIF{
		Image: make([]*image.Paletted, 0, len(plan.Frames)),
		Delay: make([]int, 0, len(plan.Frames)),
	}
	delay := (100 + plan.FPS/2) / plan.FPS
	if delay < 1 {
		delay = 1
	}
	manifest := ArtifactManifest{
		Schema:       ManifestSchema,
		TaskID:       plan.TaskID,
		SourceHash:   plan.SourceHash,
		ConfigHash:   plan.ConfigHash,
		Renderer:     rendererVersion,
		FPS:          plan.FPS,
		FramePattern: plan.FramePattern,
	}

	var width, height int
	for i, source := range plan.Frames {
		abs := filepath.Join(projectRoot, filepath.FromSlash(source))
		img, err := decodeFrame(abs)
		if err != nil {
			return ArtifactManifest{}, fmt.Errorf("decode frame %q: %w", source, err)
		}
		bounds := img.Bounds()
		if i == 0 {
			width, height = bounds.Dx(), bounds.Dy()
			if width == 0 || height == 0 {
				return ArtifactManifest{}, fmt.Errorf("frame %q has empty dimensions", source)
			}
		} else if bounds.Dx() != width || bounds.Dy() != height {
			return ArtifactManifest{}, fmt.Errorf(
				"frame %q is %dx%d, expected %dx%d",
				source, bounds.Dx(), bounds.Dy(), width, height,
			)
		}

		name := frameName(i+1, len(plan.Frames))
		file, err := writePNG(filepath.Join(frameDir, name), img)
		if err != nil {
			return ArtifactManifest{}, fmt.Errorf("write frame %q: %w", name, err)
		}
		file.Path = FrameDirectoryName + "/" + name
		manifest.Frames = append(manifest.Frames, file)

		paletted := image.NewPaletted(image.Rect(0, 0, width, height), palette.Plan9)
		copyPaletted(paletted, img)
		anim.Image = append(anim.Image, paletted)
		anim.Delay = append(anim.Delay, delay)
	}

	media, err := writeGIF(filepath.Join(staging, mediaFileName), anim)
	if err != nil {
		return ArtifactManifest{}, fmt.Errorf("write media: %w", err)
	}
	media.Path = mediaFileName
	manifest.Media = media
	manifest.FrameSetHash = frameSetHash(manifest.Frames)
	if err := writeManifest(staging, manifest); err != nil {
		return ArtifactManifest{}, err
	}
	return manifest, nil
}

func decodeFrame(name string) (image.Image, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	return img, err
}

func copyPaletted(dst *image.Paletted, src image.Image) {
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
}

func writePNG(name string, img image.Image) (ArtifactFile, error) {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return ArtifactFile{}, err
	}
	h := sha256.New()
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	encodeErr := encoder.Encode(io.MultiWriter(file, h), img)
	syncErr := file.Sync()
	closeErr := file.Close()
	if encodeErr != nil {
		return ArtifactFile{}, encodeErr
	}
	if syncErr != nil {
		return ArtifactFile{}, syncErr
	}
	if closeErr != nil {
		return ArtifactFile{}, closeErr
	}
	info, err := os.Stat(name)
	if err != nil {
		return ArtifactFile{}, err
	}
	return ArtifactFile{SHA256: hexSum(h.Sum(nil)), Size: info.Size()}, nil
}

func writeGIF(name string, anim *gif.GIF) (ArtifactFile, error) {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return ArtifactFile{}, err
	}
	h := sha256.New()
	encodeErr := gif.EncodeAll(io.MultiWriter(file, h), anim)
	syncErr := file.Sync()
	closeErr := file.Close()
	if encodeErr != nil {
		return ArtifactFile{}, encodeErr
	}
	if syncErr != nil {
		return ArtifactFile{}, syncErr
	}
	if closeErr != nil {
		return ArtifactFile{}, closeErr
	}
	info, err := os.Stat(name)
	if err != nil {
		return ArtifactFile{}, err
	}
	return ArtifactFile{SHA256: hexSum(h.Sum(nil)), Size: info.Size()}, nil
}

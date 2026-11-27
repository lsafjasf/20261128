// Package sequence writes burned-in PPM frames and their machine-readable sidecars.
package sequence

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"frameburn/internal/burnin"
	"frameburn/internal/ppm"
	"frameburn/internal/timeline"
)

const ManifestVersion = 1

type FrameRecord struct {
	Index        int64  `json:"index"`
	File         string `json:"file"`
	FrameTag     string `json:"frame_tag"`
	Timecode     string `json:"timecode"`
	Rate         string `json:"rate"`
	Label        string `json:"label"`
	ContentFrame int64  `json:"content_frame"`
	Clip         int    `json:"clip"`
	Black        bool   `json:"black"`
	Paused       bool   `json:"paused"`
}

type Manifest struct {
	Version      int              `json:"version"`
	Rate         string           `json:"rate"`
	Width        int              `json:"width"`
	Height       int              `json:"height"`
	Frames       int              `json:"frames"`
	FramePattern string           `json:"frame_pattern"`
	ContentRect  ppm.Rect         `json:"content_rect"`
	Events       []timeline.Event `json:"events"`
	FramesFile   string           `json:"frames_file"`
}

func FrameName(index int64) string {
	return fmt.Sprintf("frame_%06d.ppm", index)
}

func Write(plan *timeline.Plan, dir string) (*Manifest, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	style := burnin.DefaultStyle(&ppm.Image{Width: plan.Config.Width, Height: plan.Config.Height})
	framesPath := filepath.Join(dir, "frames.jsonl")
	framesFile, err := os.Create(framesPath)
	if err != nil {
		return nil, err
	}
	writer := bufio.NewWriter(framesFile)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	manifest := &Manifest{
		Version:      ManifestVersion,
		Rate:         plan.Rate.String(),
		Width:        plan.Config.Width,
		Height:       plan.Config.Height,
		Frames:       plan.Config.Frames,
		FramePattern: "frame_%06d.ppm",
		Events:       append([]timeline.Event(nil), plan.Events...),
		FramesFile:   "frames.jsonl",
	}
	for _, frame := range plan.Frames {
		im, err := RenderFrame(plan, frame, style)
		if err != nil {
			_ = framesFile.Close()
			return nil, fmt.Errorf("frame %d: %w", frame.Index, err)
		}
		name := FrameName(frame.Index)
		if err := im.Save(filepath.Join(dir, name)); err != nil {
			_ = framesFile.Close()
			return nil, err
		}
		record := FrameRecord{
			Index:        frame.Index,
			File:         name,
			FrameTag:     frame.Stamp.FrameTag,
			Timecode:     frame.Stamp.Timecode,
			Rate:         frame.Stamp.Rate,
			Label:        frame.Stamp.Label,
			ContentFrame: frame.ContentFrame,
			Clip:         frame.Clip,
			Black:        frame.Black,
			Paused:       frame.Paused,
		}
		if err := encoder.Encode(record); err != nil {
			_ = framesFile.Close()
			return nil, err
		}
		if manifest.ContentRect == (ppm.Rect{}) {
			manifest.ContentRect = style.ContentRect(im, frame.Stamp.Label)
		}
	}
	if err := writer.Flush(); err != nil {
		_ = framesFile.Close()
		return nil, err
	}
	if err := framesFile.Close(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		return nil, err
	}
	return manifest, nil
}

func RenderFrame(plan *timeline.Plan, frame timeline.Frame, style burnin.Style) (*ppm.Image, error) {
	im, err := renderPicture(plan, frame)
	if err != nil {
		return nil, err
	}
	panel := style.PanelRect(frame.Stamp.Label)
	if err := validatePanel(im, panel); err != nil {
		return nil, err
	}
	burnin.Burn(im, frame.Stamp, style)
	return im, nil
}

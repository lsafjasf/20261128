package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"frameburn/internal/burnin"
	"frameburn/internal/ppm"
	"frameburn/internal/sequence"
	"frameburn/internal/timeline"
)

type Options struct {
	BlackLuma      float64
	CutThreshold   float64
	PauseThreshold float64
}

func DefaultOptions() Options {
	return Options{BlackLuma: 8, CutThreshold: 28, PauseThreshold: 0.25}
}

func Audit(dir string, options Options) (*Report, error) {
	var manifest sequence.Manifest
	if err := readJSON(filepath.Join(dir, "manifest.json"), &manifest); err != nil {
		return nil, err
	}
	if manifest.Version != sequence.ManifestVersion {
		return nil, fmt.Errorf("unsupported manifest version %d", manifest.Version)
	}
	records, err := readRecords(filepath.Join(dir, manifest.FramesFile))
	if err != nil {
		return nil, err
	}
	plan, err := timeline.Build(timeline.Config{
		Rate: manifest.Rate, Width: manifest.Width, Height: manifest.Height,
		Frames: manifest.Frames, Events: manifest.Events,
	})
	if err != nil {
		return nil, fmt.Errorf("rebuild timeline: %w", err)
	}
	report := &Report{
		Dir:    dir,
		Rate:   manifest.Rate,
		Width:  manifest.Width,
		Height: manifest.Height,
		Frames: manifest.Frames,
		ContentRect: pixelRect{
			X: manifest.ContentRect.X, Y: manifest.ContentRect.Y,
			Width: manifest.ContentRect.Width, Height: manifest.ContentRect.Height,
		},
	}
	if len(records) != manifest.Frames {
		report.Errors = append(report.Errors,
			fmt.Sprintf("frames.jsonl has %d records, manifest says %d", len(records), manifest.Frames))
	}
	files, err := listFrameFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(files) != manifest.Frames {
		report.Errors = append(report.Errors,
			fmt.Sprintf("found %d PPM files, expected %d", len(files), manifest.Frames))
	}
	content := manifest.ContentRect
	if err := validateContentRect(content, manifest.Width, manifest.Height); err != nil {
		return nil, err
	}
	pictures := newPictureAnalyzer(content, options, manifest.Frames)
	for i, rec := range records {
		if i >= len(plan.Frames) {
			break
		}
		im, pixelsOK := checkFrame(dir, rec, plan.Frames[i], report)
		if im != nil {
			pictures.add(i, im)
		}
		if !pixelsOK {
			report.Labels.Mismatches = append(report.Labels.Mismatches, int(rec.Index))
		}
		report.Labels.Checked++
	}
	report.Labels.OK = len(report.Labels.Mismatches) == 0 && report.Labels.Checked == manifest.Frames
	if !report.Labels.OK {
		report.Errors = append(report.Errors, "frame/timecode label verification failed")
	}
	pictures.finish(report, plan)
	report.OK = len(report.Errors) == 0 && report.Labels.OK &&
		report.Black.OK && report.Cuts.OK && report.Pauses.OK
	return report, nil
}

func validateContentRect(r ppm.Rect, width, height int) error {
	if r.Empty() || r.X < 0 || r.Y < 0 || r.X+r.Width > width || r.Y+r.Height > height {
		return fmt.Errorf("invalid content rect %+v for %dx%d", r, width, height)
	}
	return nil
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func readRecords(path string) ([]sequence.FrameRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []sequence.FrameRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var record sequence.FrameRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}

func checkFrame(dir string, rec sequence.FrameRecord, want timeline.Frame, report *Report) (*ppm.Image, bool) {
	ok := true
	if rec.Index != want.Index {
		report.Errors = append(report.Errors, fmt.Sprintf("record index %d, expected %d", rec.Index, want.Index))
		ok = false
	}
	if rec.File != sequence.FrameName(want.Index) {
		report.Errors = append(report.Errors, fmt.Sprintf("frame %d file %q", rec.Index, rec.File))
		ok = false
	}
	if rec.FrameTag != want.Stamp.FrameTag || rec.Timecode != want.Stamp.Timecode ||
		rec.Rate != want.Stamp.Rate || rec.Label != want.Stamp.Label {
		report.Errors = append(report.Errors, fmt.Sprintf("frame %d timecode metadata drifted", rec.Index))
		ok = false
	}
	if rec.ContentFrame != want.ContentFrame || rec.Clip != want.Clip ||
		rec.Black != want.Black || rec.Paused != want.Paused {
		report.Errors = append(report.Errors, fmt.Sprintf("frame %d event metadata drifted", rec.Index))
		ok = false
	}
	im, err := ppm.Load(filepath.Join(dir, rec.File))
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("frame %d: %v", rec.Index, err))
		return nil, false
	}
	if im.Width != report.Width || im.Height != report.Height {
		report.Errors = append(report.Errors,
			fmt.Sprintf("frame %d is %dx%d, expected %dx%d", rec.Index, im.Width, im.Height, report.Width, report.Height))
		return im, false
	}
	style := burnin.DefaultStyle(im)
	panel := style.PanelRect(want.Stamp.Label)
	if panel.X < 0 || panel.Y < 0 ||
		panel.X+panel.Width > im.Width || panel.Y+panel.Height > im.Height {
		report.Errors = append(report.Errors,
			fmt.Sprintf("frame %d label panel does not fit", rec.Index))
		return im, false
	}
	labelOnly, err := ppm.New(panel.Width, panel.Height, ppm.Black)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("frame %d: %v", rec.Index, err))
		return im, false
	}
	localStyle := style
	localStyle.X -= panel.X
	localStyle.Y -= panel.Y
	burnin.Burn(labelOnly, want.Stamp, localStyle)
	for y := 0; y < panel.Height; y++ {
		for x := 0; x < panel.Width; x++ {
			if im.At(panel.X+x, panel.Y+y) != labelOnly.At(x, y) {
				return im, false
			}
		}
	}
	return im, ok
}

func listFrameFiles(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "frame_*.ppm"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

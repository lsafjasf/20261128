package releaseaudit

import (
	"fmt"
	"math"
	"strings"
)

func (a *auditor) row(name string) *Row {
	if idx, ok := a.rowIndex[name]; ok {
		return &a.report.Rows[idx]
	}
	return nil
}

func (a *auditor) checkWindow(name string, kind Kind, v *Version) {
	if row := a.row(name); row != nil {
		row.Kind = kind
	}
	start, end := strings.TrimSpace(v.Window.Start), strings.TrimSpace(v.Window.End)
	if start == "" {
		a.rowIssue(name, "", "missing_window_start", "release window start is missing")
	}
	if end == "" {
		a.rowIssue(name, "", "missing_window_end", "release window end is missing")
	}
	if channel := strings.TrimSpace(v.Window.Channel); channel != "" {
		channelKind, ok := ClassifyKind(channel)
		switch {
		case !ok:
			a.add(name, "", SeverityWarning, "unknown_window_channel",
				fmt.Sprintf("release window channel %q is not a known distribution channel", channel))
		case kind != "" && channelKind != kind:
			a.add(name, "", SeverityError, "window_version_mismatch",
				fmt.Sprintf("release window channel is %s but the version is %s", channelKind, kind))
		}
	}

	startTime, startErr := parseWindowTime(start)
	endTime, endErr := parseWindowTime(end)
	if start != "" && startErr != nil {
		a.add(name, "", SeverityError, "invalid_window_start",
			fmt.Sprintf("invalid release window start %q", start))
	}
	if end != "" && endErr != nil {
		a.add(name, "", SeverityError, "invalid_window_end",
			fmt.Sprintf("invalid release window end %q", end))
	}
	if startErr == nil && endErr == nil && startTime.After(endTime) {
		a.add(name, "", SeverityError, "invalid_release_window",
			"release window start is after its end")
	}
	if row := a.row(name); row != nil {
		row.Window = windowText(start, end)
	}
}

func windowText(start, end string) string {
	if start == "" && end == "" {
		return "—"
	}
	if start == "" {
		start = "?"
	}
	if end == "" {
		end = "?"
	}
	return start + " → " + end
}

func (a *auditor) checkVideo(name string, kind Kind, v *Version) {
	if strings.TrimSpace(v.Video.Path) == "" {
		a.rowIssue(name, "", "missing_video_path", "finished video path is missing")
	}
	a.registerAsset(name, kind, v.Video.Path, "video")

	ratioText := strings.TrimSpace(v.Video.AspectRatio)
	ratio, ratioErr := parseAspectRatio(ratioText)
	if ratioText == "" {
		a.rowIssue(name, "", "missing_aspect_ratio", "video aspect ratio is missing")
	} else if ratioErr != nil {
		a.add(name, v.Video.Path, SeverityError, "invalid_aspect_ratio",
			fmt.Sprintf("invalid video aspect ratio %q", ratioText))
	}

	if v.Video.Width < 0 || v.Video.Height < 0 {
		a.add(name, v.Video.Path, SeverityError, "invalid_video_dimensions",
			"video width and height must not be negative")
	} else if (v.Video.Width == 0) != (v.Video.Height == 0) {
		a.add(name, v.Video.Path, SeverityWarning, "incomplete_video_dimensions",
			"video width and height must be provided together")
	}
	if ratioErr == nil && v.Video.Width > 0 && v.Video.Height > 0 {
		actual := float64(v.Video.Width) / float64(v.Video.Height)
		if math.Abs(actual-ratio) > 0.02 {
			a.add(name, v.Video.Path, SeverityError, "aspect_ratio_mismatch",
				fmt.Sprintf("aspect ratio %s does not match %dx%d (%.3f:1)",
					ratioText, v.Video.Width, v.Video.Height, actual))
		}
	}

	display := ratioText
	if display == "" && v.Video.Width > 0 && v.Video.Height > 0 {
		display = fmt.Sprintf("%.3g:1", float64(v.Video.Width)/float64(v.Video.Height))
	}
	if display == "" {
		display = "—"
	}
	if v.Video.Width > 0 && v.Video.Height > 0 {
		display += fmt.Sprintf(" (%d×%d)", v.Video.Width, v.Video.Height)
	}
	if row := a.row(name); row != nil {
		row.AspectRatio = display
	}
}

func (a *auditor) checkAudio(name string, kind Kind, v *Version) {
	if strings.TrimSpace(v.Audio.Path) == "" {
		a.rowIssue(name, "", "missing_audio_path", "primary audio path is missing")
	}
	a.registerAsset(name, kind, v.Audio.Path, "audio")

	layout := strings.TrimSpace(v.Audio.Layout)
	if layout == "" {
		a.rowIssue(name, "", "missing_audio_layout", "audio layout is missing")
	}
	if v.Audio.Channels < 0 {
		a.add(name, v.Audio.Path, SeverityError, "invalid_audio_channels",
			"audio channel count must not be negative")
	}
	if channels, ok := layoutChannels(layout); ok && v.Audio.Channels > 0 && channels != v.Audio.Channels {
		a.add(name, v.Audio.Path, SeverityError, "audio_layout_mismatch",
			fmt.Sprintf("layout %s implies %d channels, manifest declares %d", layout, channels, v.Audio.Channels))
	}
	display := layout
	if display == "" {
		display = "—"
	}
	if v.Audio.Channels > 0 {
		display += fmt.Sprintf(" (%dch)", v.Audio.Channels)
	}
	if row := a.row(name); row != nil {
		row.AudioLayout = display
	}
}

func (a *auditor) checkSubtitles(name string, kind Kind, v *Version) {
	if len(v.Subtitles) == 0 {
		a.rowIssue(name, "", "missing_subtitles", "no subtitle or caption tracks are declared")
	}
	seen := map[string]bool{}
	var display []string
	for _, sub := range v.Subtitles {
		language := strings.TrimSpace(sub.Language)
		path := strings.TrimSpace(sub.Path)
		if language == "" {
			a.add(name, path, SeverityError, "missing_subtitle_language",
				"subtitle language is missing")
		}
		if path == "" {
			a.add(name, path, SeverityError, "missing_subtitle_path",
				"subtitle file path is missing")
		}
		key := strings.ToLower(language) + "\x00" + strings.ToLower(strings.TrimSpace(sub.Kind))
		if language != "" && seen[key] {
			a.add(name, path, SeverityError, "duplicate_subtitle",
				fmt.Sprintf("subtitle track %s%s is declared more than once", language, subtitleKindText(sub.Kind)))
		}
		seen[key] = true
		a.registerAsset(name, kind, path, "subtitle")

		item := language
		if item == "" {
			item = "?"
		}
		item += subtitleKindText(sub.Kind)
		display = append(display, item)
	}
	if row := a.row(name); row != nil {
		row.Subtitles = strings.Join(display, ", ")
		if row.Subtitles == "" {
			row.Subtitles = "—"
		}
	}
}

func subtitleKindText(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return ""
	}
	return " (" + kind + ")"
}

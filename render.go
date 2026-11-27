package releaseaudit

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Format selects a report representation.
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
	FormatCSV      Format = "csv"
)

// ParseFormat validates a CLI format name.
func ParseFormat(s string) (Format, error) {
	switch Format(strings.ToLower(strings.TrimSpace(s))) {
	case FormatMarkdown, "md", "":
		return FormatMarkdown, nil
	case FormatJSON:
		return FormatJSON, nil
	case FormatCSV:
		return FormatCSV, nil
	default:
		return "", fmt.Errorf("unknown format %q (want markdown, json, or csv)", s)
	}
}

// HasErrors reports whether a blocking finding exists.
func (r Report) HasErrors() bool {
	return r.Summary.Errors > 0
}

// Render serializes the report in the requested format.
func (r Report) Render(format Format) (string, error) {
	var buf bytes.Buffer
	if err := r.Write(&buf, format); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Write serializes the report to w.
func (r Report) Write(w io.Writer, format Format) error {
	if format == "" {
		format = FormatMarkdown
	}
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case FormatCSV:
		return r.writeCSV(w)
	case FormatMarkdown:
		return r.writeMarkdown(w)
	default:
		return fmt.Errorf("unknown report format %q", format)
	}
}

func (r Report) writeMarkdown(w io.Writer) error {
	title := r.Title
	if title == "" {
		title = "release package"
	}
	fmt.Fprintf(w, "# 发行矩阵：%s\n\n", escapeMarkdown(title))
	fmt.Fprintln(w, "| 版本 | 画幅 | 音频布局 | 字幕 | 发行窗口 | Manifest | 状态 |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- | --- | --- |")
	for _, row := range r.Rows {
		manifests := strings.Join(row.Manifests, "<br>")
		if manifests == "" {
			manifests = "—"
		}
		values := []string{
			row.Version,
			row.AspectRatio,
			row.AudioLayout,
			row.Subtitles,
			row.Window,
			manifests,
			row.Status,
		}
		for i := range values {
			values[i] = markdownCell(values[i])
		}
		fmt.Fprintf(w, "| %s |\n", strings.Join(values, " | "))
	}
	fmt.Fprintf(w, "\n**汇总**：%d 个版本，%d 个错误，%d 个警告，缺项 %d，交叉引用 %d。\n",
		r.Summary.Versions, r.Summary.Errors, r.Summary.Warnings,
		r.Summary.Missing, r.Summary.CrossReferences)

	fmt.Fprintln(w, "\n## 问题")
	if len(r.Issues) == 0 {
		fmt.Fprintln(w, "\n未发现问题。")
		return nil
	}
	fmt.Fprintln(w, "\n| 级别 | 代码 | 版本 | 路径 | 说明 |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- |")
	for _, issue := range r.Issues {
		version := issue.Version
		if version == "" {
			version = "—"
		}
		path := issue.Path
		if path == "" {
			path = "—"
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n",
			markdownCell(string(issue.Severity)),
			markdownCell(issue.Code),
			markdownCell(version),
			markdownCell(path),
			markdownCell(issue.Message))
	}
	return nil
}

func (r Report) writeCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	header := []string{"version", "kind", "aspectRatio", "audioLayout", "subtitles", "window", "manifests", "status", "issues"}
	if err := cw.Write(header); err != nil {
		return err
	}
	byVersion := map[string][]string{}
	for _, issue := range r.Issues {
		byVersion[issue.Version] = append(byVersion[issue.Version],
			string(issue.Severity)+":"+issue.Code)
	}
	for _, row := range r.Rows {
		record := []string{
			row.Version,
			string(row.Kind),
			row.AspectRatio,
			row.AudioLayout,
			row.Subtitles,
			row.Window,
			strings.Join(row.Manifests, "; "),
			row.Status,
			strings.Join(byVersion[row.Version], "; "),
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func markdownCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "<br>")
	s = strings.ReplaceAll(s, "\n", "<br>")
	return strings.ReplaceAll(s, "|", "\\|")
}

func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"`", "\\`",
		"*", "\\*",
		"_", "\\_",
		"[", "\\[",
		"]", "\\]",
	)
	return replacer.Replace(s)
}

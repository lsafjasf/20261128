package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"interlace-diag/internal/analyze"
)

type Writer struct {
	format  string
	out     io.Writer
	enc     *json.Encoder
	records []analyze.FrameRecord
	meta    analyze.Document
}

func NewWriter(w io.Writer, format string, doc analyze.Document) (*Writer, error) {
	format = strings.ToLower(format)
	if format == "" {
		format = "jsonl"
	}
	switch format {
	case "jsonl", "ndjson":
		format = "jsonl"
	case "json", "json-doc", "document":
		format = "json"
	case "text", "tsv":
		format = "text"
	default:
		return nil, fmt.Errorf("unsupported output format %q (use jsonl, json or text)", format)
	}
	x := &Writer{format: format, out: w, meta: doc}
	if format == "jsonl" {
		x.enc = json.NewEncoder(w)
	}
	return x, nil
}

func (w *Writer) WriteFrame(rec analyze.FrameRecord) error {
	switch w.format {
	case "jsonl":
		return w.enc.Encode(rec)
	default:
		w.records = append(w.records, rec)
		if w.format == "text" {
			return w.writeText(rec)
		}
		return nil
	}
}

func (w *Writer) Close(summary analyze.Summary) error {
	switch w.format {
	case "jsonl":
		return w.enc.Encode(summary)
	case "json":
		w.meta.Type = "document"
		if w.records == nil {
			w.records = []analyze.FrameRecord{}
		}
		w.meta.Frames = w.records
		w.meta.Summary = summary
		enc := json.NewEncoder(w.out)
		enc.SetIndent("", "  ")
		return enc.Encode(w.meta)
	default:
		_, err := fmt.Fprintf(w.out, "SUMMARY frames=%d counts=%s dominant=%s order=%s(%.2f)\n",
			summary.Frames, counts(summary.Counts), summary.Dominant,
			strings.ToUpper(summary.FieldOrder), summary.OrderConf)
		return err
	}
}

func (w *Writer) writeText(rec analyze.FrameRecord) error {
	e := rec.Evidence
	_, err := fmt.Fprintf(w.out,
		"frame=%06d t=%.3f verdict=%s conf=%.2f order=%s(%.2f) comb=%.2f edge=%.1f%% blurH=%.2f blurV=%.2f motion=%.1fpx/field:%s dup=%.1f%% smooth=%.2f\n",
		rec.Frame, rec.TimeSeconds, rec.Verdict, rec.Confidence,
		strings.ToUpper(rec.FieldOrder.Order), rec.FieldOrder.Confidence,
		e.Combing.Score, 100*e.Combing.EdgeFraction,
		e.Blur.EdgeWidthH, e.Blur.EdgeWidthV,
		e.Motion.Speed, e.Motion.Direction,
		100*e.Smoothing.LineDuplicationRate, e.Smoothing.Score)
	return err
}

func counts(c map[string]int) string {
	keys := []string{
		analyze.VerdictWeaveError, analyze.VerdictMotionBlur,
		analyze.VerdictOverSmooth, analyze.VerdictClean, analyze.VerdictInconclusive,
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if c[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", k, c[k]))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

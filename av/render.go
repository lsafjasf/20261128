package av

import (
	"fmt"
	"io"
)

// RenderTable writes the human-readable event table: decode-order views per
// track, then a presentation-order window around every scene cut.
func RenderTable(w io.Writer, m *Model, t *EventTable) {
	fmt.Fprintf(w, "== decode order: video (timescale %d) ==\n", m.Video.Timescale)
	fmt.Fprintf(w, "%-6s %10s %10s %8s %6s %s\n", "idx", "DTS ms", "CTS ms", "ctts", "dur", "flags")
	for _, e := range t.VideoDecodeOrder {
		flags := ""
		if e.Sync {
			flags = "SYNC"
			if e.DecodeIdx > 0 {
				flags += " <cut>"
			}
		}
		fmt.Fprintf(w, "%-6d %10.1f %10.1f %+8.1f %6.1f %s\n",
			e.DecodeIdx, e.DTSms, e.CTSms, e.CTSms-e.DTSms, e.DurMs, flags)
	}
	fmt.Fprintf(w, "\n== decode order: audio (timescale %d) ==\n", m.Audio.Timescale)
	fmt.Fprintf(w, "%-6s %10s %8s %8s\n", "idx", "PTS ms", "dur", "step")
	var prev float64
	for i, e := range t.AudioDecodeOrder {
		step := ""
		if i > 0 {
			step = fmt.Sprintf("%+.1f", e.CTSms-prev)
		}
		fmt.Fprintf(w, "%-6d %10.1f %8.1f %8s\n", e.DecodeIdx, e.CTSms, e.DurMs, step)
		prev = e.CTSms
	}
	for _, c := range t.Cuts {
		fmt.Fprintf(w, "\n== cut #%d @ %.1f ms (video sample %d): presentation order, +/-%.0f ms ==\n",
			c.Ordinal, c.CutPresMs, c.VideoSampleIdx, WindowMs)
		fmt.Fprintf(w, "%-4s %-6s %10s %10s %8s %s\n", "kind", "dec#", "DTS ms", "CTS ms", "dur", "flags")
		for _, e := range c.Window {
			flags := ""
			if e.Sync {
				flags = "SYNC"
			}
			fmt.Fprintf(w, "%-4s %-6d %10.1f %10.1f %8.1f %s\n",
				e.Kind, e.DecodeIdx, e.DTSms, e.CTSms, e.DurMs, flags)
		}
		fmt.Fprintf(w, "  audio PTS jump at cut: %+.1f ms | audio coverage gap in window: %.1f ms\n",
			c.AudioJumpMs, c.AudioGapMs)
		fmt.Fprintf(w, "  video DTS excess jump: %+.1f ms | elst shift: %+.1f ms\n",
			c.VideoDTSJumpMs, c.ElstShiftMs)
	}
}

// RenderDiagnosis writes the classification result and its evidence.
func RenderDiagnosis(w io.Writer, d *Diagnosis) {
	fmt.Fprintf(w, "\n== diagnosis ==\n")
	for _, e := range d.Evidence {
		fmt.Fprintf(w, "  - %s\n", e)
	}
	fmt.Fprintf(w, "  root cause: %s\n", d.Cause)
}

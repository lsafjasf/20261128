package ghostdiag

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// WriteJSON emits a stable, indented report.
func WriteJSON(w io.Writer, report *Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}

// WriteText emits a compact human-readable report. It includes every frame
// where an artifact region was classified.
func WriteText(w io.Writer, report *Report) error {
	if _, err := fmt.Fprintf(w,
		"ghostdiag: %d frames, %dx%d, %.3f fps\ninput_sha256: %s\n",
		report.Input.Frames, report.Input.Width, report.Input.Height,
		report.Input.FrameRate, report.Input.SHA256); err != nil {
		return err
	}
	if report.FirstGhost == nil {
		if _, err := fmt.Fprintln(w, "first_ghost: none"); err != nil {
			return err
		}
	} else {
		first := report.FirstGhost
		if _, err := fmt.Fprintf(w,
			"first_ghost: frame=%d time=%.3fs region=x%d,y%d,w%d,h%d area=%d score=%.3f\n",
			first.Frame, first.Timestamp, first.Region.X, first.Region.Y,
			first.Region.Width, first.Region.Height, first.Region.Area, first.Score); err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(report.Summary.Counts))
	for key := range report.Summary.Counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := fmt.Fprintf(w, "count.%s: %d\n", key, report.Summary.Counts[key]); err != nil {
			return err
		}
	}
	for _, frame := range report.Frames {
		if len(frame.Findings) == 0 {
			continue
		}
		for _, finding := range frame.Findings {
			secondary := ""
			if len(finding.Secondary) > 0 {
				secondary = fmt.Sprintf(" secondary=%v", finding.Secondary)
			}
			if _, err := fmt.Fprintf(w,
				"frame=%d t=%.3f class=%s region=x%d,y%d,w%d,h%d score=%.3f temporal=%.3f block=%.3f blur=%.2f%s\n",
				finding.Frame, finding.Timestamp, finding.Classification,
				finding.Region.X, finding.Region.Y, finding.Region.Width,
				finding.Region.Height, finding.Score, finding.Evidence.TemporalScore,
				finding.Evidence.BlockScore, finding.Evidence.BlurWidth, secondary); err != nil {
				return err
			}
		}
	}
	return nil
}

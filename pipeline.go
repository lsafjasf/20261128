package subburn

import (
	"fmt"
	"time"
)

type Pipeline struct {
	Duration time.Duration
	Preview  RenderClock
	Export   RenderClock
}

type Comparison struct {
	CueID            string
	Expected         time.Duration
	Observed         time.Duration
	Error            time.Duration
	PreviewQueryTime time.Duration
	ExportQueryTime  time.Duration
	PreviewFrame     int64
	ExportFrame      int64
}

func (p Pipeline) Validate() error {
	if p.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	if err := p.Preview.Validate(); err != nil {
		return fmt.Errorf("preview: %w", err)
	}
	if err := p.Export.Validate(); err != nil {
		return fmt.Errorf("export: %w", err)
	}
	return nil
}

// CompareCues compares the actual display onset of every cue in the preview
// and export clocks. The error is Observed-Expected, so a negative value means
// the burned-in subtitle appears early.
func (p Pipeline) CompareCues(cues []Cue) ([]Comparison, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	comparisons := make([]Comparison, 0, len(cues))
	for _, cue := range cues {
		if err := cue.Validate(); err != nil {
			return nil, err
		}
		if cue.End > p.Duration {
			return nil, fmt.Errorf("cue %q ends after media duration", cue.ID)
		}
		previewSpan, ok := p.Preview.CueSpan(cue, p.Duration)
		if !ok {
			return nil, fmt.Errorf("cue %q is not visible in preview", cue.ID)
		}
		exportSpan, ok := p.Export.CueSpan(cue, p.Duration)
		if !ok {
			return nil, fmt.Errorf("cue %q is not visible in export", cue.ID)
		}
		expected := p.Preview.TrueTime(previewSpan.First)
		observed := p.Export.TrueTime(exportSpan.First)
		comparisons = append(comparisons, Comparison{
			CueID:            cue.ID,
			Expected:         expected,
			Observed:         observed,
			Error:            observed - expected,
			PreviewQueryTime: p.Preview.QueryTime(previewSpan.First),
			ExportQueryTime:  p.Export.QueryTime(exportSpan.First),
			PreviewFrame:     previewSpan.First,
			ExportFrame:      exportSpan.First,
		})
	}
	return comparisons, nil
}

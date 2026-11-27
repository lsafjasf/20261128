// Package audit verifies burned-in PPM sequences against their manifest.
package audit

type Run struct {
	Start int `json:"start"`
	Count int `json:"count"`
}

type LabelCheck struct {
	OK         bool  `json:"ok"`
	Checked    int   `json:"checked"`
	Mismatches []int `json:"mismatches,omitempty"`
}

type BlackCheck struct {
	OK         bool  `json:"ok"`
	Expected   []Run `json:"expected"`
	Detected   []Run `json:"detected"`
	Missing    []Run `json:"missing,omitempty"`
	Unexpected []Run `json:"unexpected,omitempty"`
}

type CutCheck struct {
	OK         bool  `json:"ok"`
	Expected   []int `json:"expected"`
	Detected   []int `json:"detected"`
	Missing    []int `json:"missing,omitempty"`
	Unexpected []int `json:"unexpected,omitempty"`
}

type PauseCheck struct {
	OK         bool  `json:"ok"`
	Expected   []Run `json:"expected"`
	Detected   []Run `json:"detected"`
	Missing    []Run `json:"missing,omitempty"`
	Unexpected []Run `json:"unexpected,omitempty"`
}

type Report struct {
	OK           bool       `json:"ok"`
	Dir          string     `json:"dir"`
	Rate         string     `json:"rate"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	Frames       int        `json:"frames"`
	ContentRect  pixelRect  `json:"content_rect"`
	Labels       LabelCheck `json:"labels"`
	Black        BlackCheck `json:"black"`
	Cuts         CutCheck   `json:"cuts"`
	Pauses       PauseCheck `json:"pauses"`
	Errors       []string   `json:"errors,omitempty"`
	MaxCutDiff   float64    `json:"max_cut_diff"`
	MaxPauseDiff float64    `json:"max_pause_diff"`
}

type pixelRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (r Run) equal(other Run) bool {
	return r.Start == other.Start && r.Count == other.Count
}

func containsRun(runs []Run, want Run) bool {
	for _, run := range runs {
		if run.equal(want) {
			return true
		}
	}
	return false
}

func missingRuns(expected, detected []Run) []Run {
	var missing []Run
	for _, run := range expected {
		if !containsRun(detected, run) {
			missing = append(missing, run)
		}
	}
	return missing
}

func unexpectedRuns(expected, detected []Run) []Run {
	var unexpected []Run
	for _, run := range detected {
		if !containsRun(expected, run) {
			unexpected = append(unexpected, run)
		}
	}
	return unexpected
}

package audit

import (
	"sort"

	"frameburn/internal/ppm"
	"frameburn/internal/timeline"
)

type pictureAnalyzer struct {
	content       ppm.Rect
	options       Options
	total         int
	blackFlags    []bool
	pausedFlags   []bool
	cuts          []int
	previous      *ppm.Image
	previousBlack bool
	lastIndex     int
	maxDiff       float64
	maxPauseDiff  float64
}

func newPictureAnalyzer(content ppm.Rect, options Options, total int) *pictureAnalyzer {
	if options.CutThreshold <= 0 {
		options.CutThreshold = DefaultOptions().CutThreshold
	}
	return &pictureAnalyzer{
		content:     content,
		options:     options,
		total:       total,
		blackFlags:  make([]bool, total),
		pausedFlags: make([]bool, total),
		lastIndex:   -2,
	}
}

func (p *pictureAnalyzer) add(index int, im *ppm.Image) {
	if index < 0 || index >= p.total || im == nil {
		return
	}
	black := meanLuma(im, p.content) <= p.options.BlackLuma
	p.blackFlags[index] = black
	if p.previous != nil && index == p.lastIndex+1 {
		diff := meanDiff(p.previous, im, p.content)
		p.maxDiff = maxFloat(p.maxDiff, diff)
		if !p.previousBlack && !black {
			if diff <= p.options.PauseThreshold {
				p.pausedFlags[index] = true
				p.maxPauseDiff = maxFloat(p.maxPauseDiff, diff)
				if index == 1 {
					p.pausedFlags[0] = true
				}
			}
			if diff >= p.options.CutThreshold {
				p.cuts = append(p.cuts, index)
			}
		}
	}
	p.previous = im
	p.previousBlack = black
	p.lastIndex = index
}

func (p *pictureAnalyzer) finish(report *Report, plan *timeline.Plan) {
	report.MaxCutDiff = p.maxDiff
	report.MaxPauseDiff = p.maxPauseDiff
	report.Black.Expected = runsFromMap(plan.BlackSet, p.total)
	report.Black.Detected = runsFromSlice(p.blackFlags, p.total)
	report.Black.Missing = missingRuns(report.Black.Expected, report.Black.Detected)
	report.Black.Unexpected = unexpectedRuns(report.Black.Expected, report.Black.Detected)
	report.Black.OK = len(report.Black.Missing) == 0 && len(report.Black.Unexpected) == 0
	if !report.Black.OK {
		report.Errors = append(report.Errors, "black-frame runs differ from manifest")
	}

	report.Pauses.Expected = runsFromMap(plan.PauseSet, p.total)
	report.Pauses.Detected = runsFromSlice(p.pausedFlags, p.total)
	report.Pauses.Missing = missingRuns(report.Pauses.Expected, report.Pauses.Detected)
	report.Pauses.Unexpected = unexpectedRuns(report.Pauses.Expected, report.Pauses.Detected)
	report.Pauses.OK = len(report.Pauses.Missing) == 0 && len(report.Pauses.Unexpected) == 0
	if !report.Pauses.OK {
		report.Errors = append(report.Errors, "pause/freeze runs differ from manifest")
	}

	report.Cuts.Expected = intsFromSet(plan.CutSet)
	report.Cuts.Detected = p.cuts
	report.Cuts.Missing = missingInts(report.Cuts.Expected, report.Cuts.Detected)
	report.Cuts.Unexpected = unexpectedInts(report.Cuts.Expected, report.Cuts.Detected)
	report.Cuts.OK = len(report.Cuts.Missing) == 0 && len(report.Cuts.Unexpected) == 0
	if !report.Cuts.OK {
		report.Errors = append(report.Errors, "edit points differ from manifest")
	}
	p.previous = nil
}

func meanLuma(im *ppm.Image, r ppm.Rect) float64 {
	if r.Empty() {
		return 0
	}
	var sum uint64
	for y := r.Y; y < r.Y+r.Height; y++ {
		for x := r.X; x < r.X+r.Width; x++ {
			c := im.At(x, y)
			sum += uint64(299)*uint64(c.R) + uint64(587)*uint64(c.G) + uint64(114)*uint64(c.B)
		}
	}
	return float64(sum) / (1000 * float64(r.Width*r.Height))
}

func meanDiff(a, b *ppm.Image, r ppm.Rect) float64 {
	if r.Empty() {
		return 0
	}
	var sum uint64
	for y := r.Y; y < r.Y+r.Height; y++ {
		for x := r.X; x < r.X+r.Width; x++ {
			ca, cb := a.At(x, y), b.At(x, y)
			sum += absDiff(ca.R, cb.R) + absDiff(ca.G, cb.G) + absDiff(ca.B, cb.B)
		}
	}
	return float64(sum) / (3 * float64(r.Width*r.Height))
}

func absDiff(a, b uint8) uint64 {
	if a > b {
		return uint64(a - b)
	}
	return uint64(b - a)
}

func runsFromMap(flags map[int]bool, n int) []Run {
	var runs []Run
	for i := 0; i < n; i++ {
		if !flags[i] {
			continue
		}
		start := i
		for i+1 < n && flags[i+1] {
			i++
		}
		runs = append(runs, Run{Start: start, Count: i - start + 1})
	}
	return runs
}

func runsFromSlice(flags []bool, n int) []Run {
	var runs []Run
	for i := 0; i < n && i < len(flags); i++ {
		if !flags[i] {
			continue
		}
		start := i
		for i+1 < n && flags[i+1] {
			i++
		}
		runs = append(runs, Run{Start: start, Count: i - start + 1})
	}
	return runs
}

func intsFromSet(set map[int]bool) []int {
	var values []int
	for value := range set {
		values = append(values, value)
	}
	sort.Ints(values)
	return values
}

func missingInts(expected, detected []int) []int {
	var missing []int
	for _, want := range expected {
		found := false
		for _, got := range detected {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, want)
		}
	}
	return missing
}

func unexpectedInts(expected, detected []int) []int {
	var unexpected []int
	for _, got := range detected {
		found := false
		for _, want := range expected {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			unexpected = append(unexpected, got)
		}
	}
	return unexpected
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

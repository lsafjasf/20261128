package analyze

import (
	"fmt"

	"interlace-diag/internal/video"
)

type Options struct {
	MaxShiftX     int
	MaxShiftY     int
	DeclaredOrder string
}

type frameData struct {
	frame       video.Frame
	full        plane
	top         plane
	bottom      plane
	intra       Shift
	inter       Shift
	effective   Shift
	comb        CombEvidence
	blur        BlurEvidence
	smoothing   SmoothingEvidence
	fields      FieldEvidence
	temporalMAD float64
}

type Analyzer struct {
	opts      Options
	prev      *frameData
	width     int
	height    int
	order     string
	orderConf float64
	counts    map[string]int
	frames    int
}

func NewAnalyzer(opts Options) *Analyzer {
	opts.DeclaredOrder = normalizeOrder(opts.DeclaredOrder)
	return &Analyzer{opts: opts, counts: make(map[string]int)}
}

func normalizeOrder(v string) string {
	switch v {
	case "tff", "top", "top-first", "top_first":
		return "tff"
	case "bff", "bottom", "bottom-first", "bottom_first":
		return "bff"
	default:
		return ""
	}
}

func (a *Analyzer) Push(f video.Frame) (*FrameRecord, error) {
	if f.Width <= 0 || f.Height <= 0 || len(f.Y) < f.Width*f.Height {
		return nil, fmt.Errorf("frame %d has invalid dimensions or luma size", f.Index)
	}
	if a.width == 0 {
		a.width, a.height = f.Width, f.Height
	} else if f.Width != a.width || f.Height != a.height {
		return nil, fmt.Errorf("frame %d changed dimensions from %dx%d to %dx%d",
			f.Index, a.width, a.height, f.Width, f.Height)
	}
	d := a.measure(f)
	if a.prev == nil {
		a.prev = d
		return nil, nil
	}
	d.temporalMAD = planeMAD(d.frame.Y, a.prev.frame.Y)
	a.prev.temporalMAD = d.temporalMAD
	maxX, maxY := a.shiftLimits(d.frame.Width, d.frame.Height)
	inter := estimateShift(a.prev.full, d.full, maxX*2, maxY*2)
	a.prev.inter, a.prev.effective = inter, effectiveMotion(a.prev.intra, inter)
	d.inter, d.effective = inter, effectiveMotion(d.intra, inter)
	a.prev.smoothing.Score = smoothingScore(a.prev.smoothing, a.prev.blur, a.prev.effective)
	d.smoothing.Score = smoothingScore(d.smoothing, d.blur, d.effective)

	order, conf, source := a.resolveOrder(a.prev, d)
	rec := a.makeRecord(*a.prev, order, conf, source)
	a.prev = d
	return &rec, nil
}

func (a *Analyzer) Flush() *FrameRecord {
	if a.prev == nil {
		return nil
	}
	order, conf, source := a.order, a.orderConf, "sequence_prior"
	if order == "" && a.opts.DeclaredOrder != "" {
		order, conf, source = a.opts.DeclaredOrder, 0.35, "declared"
	}
	if order == "" {
		source = "unavailable"
	}
	rec := a.makeRecord(*a.prev, order, conf, source)
	a.prev = nil
	return &rec
}

func (a *Analyzer) measure(f video.Frame) *frameData {
	top := extractField(f, 0)
	bottom := extractField(f, 1)
	top.prepareProfiles()
	bottom.prepareProfiles()
	full := plane{W: f.Width, H: f.Height, Pix: f.Y}
	full.prepareProfiles()
	maxX, maxY := a.shiftLimits(f.Width, f.Height)
	d := &frameData{
		frame:  f,
		full:   full,
		top:    top,
		bottom: bottom,
		intra:  estimateShift(top, bottom, maxX, maxY),
		comb:   measureComb(f),
		blur:   measureBlur([]plane{top, bottom}),
	}
	d.effective = d.intra
	var temporalDup float64
	d.fields, temporalDup = measureFields(top, bottom, d.intra)
	d.smoothing.LineDuplicationRate, d.smoothing.VerticalDetailRatio = measureLineDuplication(f)
	d.smoothing.TemporalDuplication = temporalDup
	d.smoothing.Score = smoothingScore(d.smoothing, d.blur, d.intra)
	return d
}

func (a *Analyzer) shiftLimits(w, h int) (int, int) {
	maxX, maxY := a.opts.MaxShiftX, a.opts.MaxShiftY
	if maxX <= 0 {
		maxX = minInt(40, maxInt(8, w/8))
	}
	if maxY <= 0 {
		maxY = minInt(18, maxInt(3, h/40))
	}
	maxX = minInt(maxX, maxInt(1, w/2-1))
	maxY = minInt(maxY, maxInt(1, h/2-1))
	return maxX, maxY
}

func (a *Analyzer) resolveOrder(prev, cur *frameData) (string, float64, string) {
	maxX, maxY := a.shiftLimits(prev.frame.Width, prev.frame.Height)
	order, conf := chooseFieldOrder(prev.top, prev.bottom, cur.top, cur.bottom, maxX, maxY)
	if order != "" {
		a.order, a.orderConf = order, conf
		return order, conf, "measured"
	}
	if a.order != "" {
		return a.order, a.orderConf * 0.75, "sequence_prior"
	}
	if a.opts.DeclaredOrder != "" {
		return a.opts.DeclaredOrder, 0.35, "declared"
	}
	return "", 0, "insufficient_motion"
}

func (a *Analyzer) makeRecord(d frameData, order string, orderConf float64, orderSource string) FrameRecord {
	verdict, confidence, scores, reasons := classify(d, order)
	a.counts[verdict]++
	a.frames++
	return FrameRecord{
		Type:        "frame",
		Frame:       d.frame.Index,
		TimeSeconds: d.frame.TimeSeconds,
		Verdict:     verdict,
		Confidence:  confidence,
		FieldOrder: FieldOrderEvidence{
			Order:      order,
			Confidence: clamp01(orderConf),
			Source:     orderSource,
			Declared:   a.opts.DeclaredOrder,
		},
		Scores: scores,
		Evidence: FrameEvidence{
			Combing:   d.comb,
			Blur:      d.blur,
			Smoothing: d.smoothing,
			Fields:    d.fields,
			Motion: MotionEvidence{
				IntraField: d.intra,
				InterFrame: d.inter,
				Speed:      d.effective.Speed,
				Direction:  d.effective.Direction,
				Confidence: d.effective.Confidence,
			},
		},
		Reasons: reasons,
	}
}

func effectiveMotion(intra, inter Shift) Shift {
	if intra.Speed >= 0.75 && intra.Confidence >= 0.10 {
		return intra
	}
	if inter.Speed >= 1.5 && inter.Confidence >= 0.10 {
		return Shift{
			X:          inter.X / 2,
			Y:          inter.Y / 2,
			Speed:      inter.Speed / 2,
			Direction:  inter.Direction,
			Confidence: inter.Confidence,
		}
	}
	return intra
}

func (a *Analyzer) Summary() Summary {
	counts := make(map[string]int, len(a.counts))
	for k, v := range a.counts {
		counts[k] = v
	}
	order, conf := a.order, a.orderConf
	if order == "" && a.opts.DeclaredOrder != "" {
		order, conf = a.opts.DeclaredOrder, 0.35
	}
	return Summary{
		Type:       "summary",
		Frames:     a.frames,
		Counts:     counts,
		Dominant:   dominant(a.counts),
		FieldOrder: order,
		OrderConf:  clamp01(conf),
	}
}

func dominant(counts map[string]int) string {
	order := []string{VerdictWeaveError, VerdictMotionBlur, VerdictOverSmooth, VerdictClean, VerdictInconclusive}
	best, n := "", -1
	for _, key := range order {
		if counts[key] > n {
			best, n = key, counts[key]
		}
	}
	if n <= 0 {
		return ""
	}
	return best
}

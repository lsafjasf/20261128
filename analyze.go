package main

import (
	"math"
	"sort"
)

// SegmentLen is the analysis window used for DC/trend estimation.
const SegmentLen = 0.5 // seconds

type SegmentStat struct {
	Index     int     `json:"index"`
	StartSec  float64 `json:"start_sec"`
	Mean      float64 `json:"mean"` // segment DC estimate
	PeakPos   float64 `json:"peak_pos"`
	PeakNeg   float64 `json:"peak_neg"`
	Subsonic  float64 `json:"subsonic_rms"` // RMS of <20Hz residual after detrend
}

type ChannelAnalysis struct {
	Channel        int           `json:"channel"`
	DCOffset       float64       `json:"dc_offset"`       // robust (median of segment means)
	DCOffsetDBFS   float64       `json:"dc_offset_dbfs"`
	DriftPerSec    float64       `json:"drift_per_sec"`   // linear slope of segment means
	DriftTotal     float64       `json:"drift_total"`     // slope * duration
	TrendResidual  float64       `json:"trend_residual"`  // RMS of means around linear fit
	SubsonicRMS    float64       `json:"subsonic_rms"`    // true ultra-low-freq energy
	Classification string        `json:"classification"`  // fixed|drift|subsonic|mixed|clean
	PeakPos        float64       `json:"peak_pos"`
	PeakNeg        float64       `json:"peak_neg"`
	HeadroomPosDB  float64       `json:"headroom_pos_db"` // to +full scale
	HeadroomNegDB  float64       `json:"headroom_neg_db"` // to -full scale
	EffectiveDB    float64       `json:"effective_headroom_db"` // min of the two: real usable margin
	Segments       []SegmentStat `json:"segments"`
}

// AnalyzeChannel estimates DC offset, drift slope, subsonic residue and
// asymmetric headroom for one channel.
func AnalyzeChannel(ch int, x []float64, sampleRate int) ChannelAnalysis {
	a := ChannelAnalysis{Channel: ch}
	n := len(x)
	if n == 0 {
		a.Classification = "clean"
		return a
	}
	segLen := int(SegmentLen * float64(sampleRate))
	if segLen < 64 {
		segLen = 64
	}
	nSeg := (n + segLen - 1) / segLen
	means := make([]float64, 0, nSeg)
	times := make([]float64, 0, nSeg)

	for s := 0; s < nSeg; s++ {
		lo, hi := s*segLen, (s+1)*segLen
		if hi > n {
			hi = n
		}
		st := SegmentStat{Index: s, StartSec: float64(lo) / float64(sampleRate), PeakNeg: math.Inf(1)}
		var sum float64
		for i := lo; i < hi; i++ {
			v := x[i]
			sum += v
			if v > st.PeakPos {
				st.PeakPos = v
			}
			if v < st.PeakNeg {
				st.PeakNeg = v
			}
		}
		st.Mean = sum / float64(hi-lo)
		st.Subsonic = subsonicRMS(x[lo:hi], sampleRate)
		a.Segments = append(a.Segments, st)
		means = append(means, st.Mean)
		times = append(times, st.StartSec)
		if st.PeakPos > a.PeakPos {
			a.PeakPos = st.PeakPos
		}
		if st.PeakNeg < a.PeakNeg || s == 0 {
			a.PeakNeg = st.PeakNeg
		}
	}

	a.DCOffset = median(means)
	a.DCOffsetDBFS = toDB(math.Abs(a.DCOffset))
	slope, residual := linfit(times, means)
	a.DriftPerSec = slope
	a.DriftTotal = slope * float64(n) / float64(sampleRate)
	a.TrendResidual = residual
	for _, st := range a.Segments {
		a.SubsonicRMS += st.Subsonic * st.Subsonic
	}
	a.SubsonicRMS = math.Sqrt(a.SubsonicRMS / float64(len(a.Segments)))

	// Asymmetric headroom in dB below full scale (positive = margin).
	// A plain peak meter only reports max(|peak|), hiding the worse side.
	a.HeadroomPosDB = -toDB(a.PeakPos)
	a.HeadroomNegDB = -toDB(math.Abs(a.PeakNeg))
	a.EffectiveDB = math.Min(a.HeadroomPosDB, a.HeadroomNegDB)
	a.Classification = classify(a)
	return a
}

const (
	dcEps       = 1e-3 // ~ -66 dBFS: below this, offset is negligible
	driftEps    = 3e-3 // total drift over file that matters
	subsonicEps = 5e-4
)

func classify(a ChannelAnalysis) string {
	fixed := math.Abs(a.DCOffset) > dcEps
	drift := math.Abs(a.DriftTotal) > driftEps && a.TrendResidual < math.Abs(a.DriftTotal)*0.5
	sub := a.SubsonicRMS > subsonicEps
	var parts []string
	if fixed {
		parts = append(parts, "fixed")
	}
	if drift {
		parts = append(parts, "drift")
	}
	if sub {
		parts = append(parts, "subsonic")
	}
	if len(parts) == 0 {
		return "clean"
	}
	return joinParts(parts)
}

func joinParts(p []string) string {
	s := p[0]
	for _, q := range p[1:] {
		s += "+" + q
	}
	return s
}

// subsonicRMS estimates true ultra-low-frequency energy: extract the
// <20Hz band with a zero-phase low-pass, remove its best-fit line (so
// DC and linear drift do not count), and return the residual RMS.
func subsonicRMS(x []float64, sampleRate int) float64 {
	if len(x) < 8 {
		return 0
	}
	lf := ZeroPhaseLowPass(x, 20, float64(sampleRate))
	// Skip edge warm-up (~100ms, several time constants of the 20Hz
	// corner) so the zero-state transient is not taken for subsonic.
	warmup := sampleRate / 10
	if 2*warmup >= len(lf) {
		warmup = len(lf) / 4
	}
	// Fit the DC+drift line over the kept region only; edge transients
	// would otherwise corrupt the fit.
	n := float64(len(lf))
	var st, sy, stt, sty float64
	var cnt int
	for i := warmup; i < len(lf)-warmup; i++ {
		t := float64(i) / n
		st += t
		sy += lf[i]
		stt += t * t
		sty += t * lf[i]
		cnt++
	}
	cn := float64(cnt)
	den := cn*stt - st*st
	var slope, intercept float64
	if den != 0 {
		slope = (cn*sty - st*sy) / den
		intercept = (sy - slope*st) / cn
	}
	var sq float64
	cnt = 0
	for i, v := range lf {
		if i < warmup || i >= len(lf)-warmup {
			continue
		}
		t := float64(i) / n
		r := v - (intercept + slope*t)
		sq += r * r
		cnt++
	}
	if cnt == 0 {
		return 0
	}
	return math.Sqrt(sq / float64(cnt))
}

func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// linfit returns slope and residual RMS of y ~ a + b*t.
func linfit(t, y []float64) (slope, residRMS float64) {
	n := float64(len(t))
	var st, sy, stt, sty float64
	for i := range t {
		st += t[i]
		sy += y[i]
		stt += t[i] * t[i]
		sty += t[i] * y[i]
	}
	den := n*stt - st*st
	if den == 0 {
		return 0, 0
	}
	slope = (n*sty - st*sy) / den
	intercept := (sy - slope*st) / n
	var sq float64
	for i := range t {
		r := y[i] - (intercept + slope*t[i])
		sq += r * r
	}
	return slope, math.Sqrt(sq / n)
}

func toDB(v float64) float64 {
	if v <= 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(v)
}

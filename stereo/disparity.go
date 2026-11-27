package stereo

// Budget is the safe screen-parallax envelope, in pixels. Parallax inside
// [MaxCrossed, MaxUncrossed] reads as normal stereo; outside it risks eye
// strain. MaxCrossed is negative (objects in front of the screen force
// convergence); MaxUncrossed is positive (objects behind the screen; past
// the interocular-equivalent the eyes must diverge, which is the main
// fatigue driver).
type Budget struct {
	MaxCrossed   float64 // most negative allowed parallax (px), e.g. -40
	MaxUncrossed float64 // most positive allowed parallax (px), e.g. +24
}

// DefaultBudget derives a conservative envelope from the rig: uncrossed is
// capped at the interocular-equivalent on screen (divergence limit), crossed
// at 1.5x that. Either is additionally capped at 3% of screen width.
func (r Rig) DefaultBudget() Budget {
	ioPx := r.Interaxial * r.PixelsPerUnit
	capPct := 0.03 * r.ScreenWidthPx
	uncrossed := min(ioPx, capPct)
	crossed := max(-1.5*ioPx, -capPct)
	return Budget{MaxCrossed: crossed, MaxUncrossed: uncrossed}
}

// Level classifies one parallax value against a Budget.
type Level int

const (
	// LevelOK is comfortable, normal stereo.
	LevelOK Level = iota
	// LevelStrain is outside the safe envelope: eye-strain risk.
	LevelStrain
)

func (l Level) String() string {
	if l == LevelOK {
		return "ok"
	}
	return "strain-risk"
}

// classifyEps absorbs floating-point noise so a value clamped to a budget
// boundary still classifies as safe.
const classifyEps = 1e-9

// Classify reports whether parallaxPx is within the safe budget.
func (b Budget) Classify(parallaxPx float64) Level {
	if parallaxPx < b.MaxCrossed-classifyEps || parallaxPx > b.MaxUncrossed+classifyEps {
		return LevelStrain
	}
	return LevelOK
}

// Clamp pulls parallaxPx back to the nearest safe boundary.
func (b Budget) Clamp(parallaxPx float64) float64 {
	return min(max(parallaxPx, b.MaxCrossed), b.MaxUncrossed)
}

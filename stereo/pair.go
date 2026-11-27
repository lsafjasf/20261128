package stereo

// Pair is one stereoscopic subtitle cue: identical text for both eyes, placed
// at LeftX in the left-eye image and RightX in the right-eye image (screen
// pixels). The horizontal gap is the disparity that sets perceived depth.
type Pair struct {
	Text   string
	LeftX  float64
	RightX float64
}

// Disparity is RightX - LeftX in pixels (positive = behind the screen),
// matching Rig.ParallaxPx.
func (p Pair) Disparity() float64 { return p.RightX - p.LeftX }

// Center is the midpoint of the two eye positions: the on-screen anchor.
func (p Pair) Center() float64 { return (p.LeftX + p.RightX) / 2 }

// Depth is the perceived z of the pair under the given rig.
func (p Pair) Depth(r Rig) float64 { return r.DepthForParallax(p.Disparity()) }

// Level classifies the pair's disparity against the budget.
func (p Pair) Level(b Budget) Level { return b.Classify(p.Disparity()) }

// MoveLeft shifts the left-eye placement by dx and synchronously shifts the
// right eye by the same dx, so depth (disparity) is preserved: the subtitle
// slides on screen without changing its stereo depth.
func (p *Pair) MoveLeft(dx float64) {
	p.LeftX += dx
	p.RightX += dx
}

// MoveRight is the mirror of MoveLeft: adjusting the right eye moves the
// left eye by the same dx. Both eyes always stay in sync.
func (p *Pair) MoveRight(dx float64) { p.MoveLeft(dx) }

// FixDisparity brings an out-of-budget pair back into the safe envelope.
// The disparity is clamped to the budget and the correction is applied
// symmetrically to BOTH eyes around the pair center, so the on-screen
// anchor does not drift and the two eyes never disagree.
// It reports whether a correction was needed.
func (p *Pair) FixDisparity(b Budget) bool {
	d := p.Disparity()
	clamped := b.Clamp(d)
	if clamped == d {
		return false
	}
	center := p.Center()
	p.LeftX = center - clamped/2
	p.RightX = center + clamped/2
	return true
}

// NewPairAtDepth builds a pair centered at centerX whose disparity yields
// depth z under the rig: the inverse of Pair.Depth.
func NewPairAtDepth(text string, centerX, z float64, r Rig) Pair {
	left, right := r.EyePairX(centerX/r.PixelsPerUnit, z)
	return Pair{
		Text:   text,
		LeftX:  left * r.PixelsPerUnit,
		RightX: right * r.PixelsPerUnit,
	}
}

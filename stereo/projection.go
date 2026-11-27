// Package stereo models stereoscopic (left/right eye) subtitle placement.
//
// Geometry: two virtual eyes sit on the x-axis at +/- Interaxial/2 and look
// down +z. The screen (zero-parallax plane) is at z = Convergence. A point at
// depth z projects from each eye onto the screen plane; the horizontal gap
// between the two projections is the screen parallax (disparity).
package stereo

// Rig describes the viewing/projection geometry. All physical lengths share
// one unit (e.g. meters); pixels enter only through PixelsPerUnit.
type Rig struct {
	Interaxial    float64 // eye separation (viewer interocular or camera io)
	Convergence   float64 // z of the zero-parallax (screen) plane
	PixelsPerUnit float64 // screen pixels per physical unit at the screen
	ScreenWidthPx float64 // full screen width in pixels
}

// ProjectX projects a point at depth z and world x onto the screen plane as
// seen from an eye at eyeX, returning the screen-plane x in world units.
func (r Rig) ProjectX(eyeX, x, z float64) float64 {
	return eyeX + (x-eyeX)*r.Convergence/z
}

// EyePairX returns the left- and right-eye screen-plane x (world units) for a
// point at (x, z). Left eye is at -Interaxial/2, right eye at +Interaxial/2.
func (r Rig) EyePairX(x, z float64) (left, right float64) {
	half := r.Interaxial / 2
	return r.ProjectX(-half, x, z), r.ProjectX(half, x, z)
}

// ParallaxPx is the on-screen disparity in pixels for a point at (x, z):
// right-eye projection minus left-eye projection. Positive means the point
// sits behind the screen (uncrossed); negative means in front (crossed).
func (r Rig) ParallaxPx(x, z float64) float64 {
	left, right := r.EyePairX(x, z)
	return (right - left) * r.PixelsPerUnit
}

// DepthForParallax inverts ParallaxPx: given a screen parallax in pixels,
// return the depth z that produces it. Parallax is x-independent for this
// parallel-eye rig, so depth depends on disparity alone.
func (r Rig) DepthForParallax(parallaxPx float64) float64 {
	// p = io*(1 - c/z)*ppu  =>  z = c / (1 - p/(io*ppu))
	denom := 1 - parallaxPx/(r.Interaxial*r.PixelsPerUnit)
	return r.Convergence / denom
}
